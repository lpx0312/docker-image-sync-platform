package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// 本文件新增与镜像仓库配置/台账维护相关的 CLI 命令：
// dsync acr test、dsync repo import / sync-records / clean

// acrTestCmd dsync acr test [别名]：测试仓库配置连通性
var acrTestCmd = &cobra.Command{
	Use:   "test [别名]",
	Short: "测试镜像仓库配置连通性（登录凭证 + SWR/CCR 管理面凭证）",
	Long: `测试镜像仓库配置是否可用：
- 登录凭证（docker login 用，推送/拉取/Tag 查询）必测；
- 华为云 SWR / 腾讯云 CCR 额外测试管理面凭证（AK/SK 或 SecretId/Key，"从仓库导入"用）。

不带参数时测试所有已配置的仓库；带别名时只测试该仓库。`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()

		regs, err := listRegistries(client)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			reg, err := resolveAcrByNamespace(client, args[0])
			if err != nil {
				return err
			}
			regs = []AcrRegistryInfo{*reg}
		}

		type testRow struct {
			Alias        string `json:"alias"`
			RegistryType string `json:"registry_type"`
			LoginOK      bool   `json:"login_ok"`
			LoginMessage string `json:"login_message"`
			ManageOK     bool   `json:"manage_ok"`
			ManageMsg    string `json:"manage_message"`
			ManageSkip   bool   `json:"manage_skipped"`
		}
		rows := make([]testRow, 0, len(regs))
		for _, reg := range regs {
			var resp registryTestResponse
			if err := client.do("POST", fmt.Sprintf("/acr-registries/%d/test", reg.ID), nil, &resp); err != nil {
				return fmt.Errorf("测试 %s 失败: %w", aliasOrNamespace(reg.Alias, reg.Namespace), err)
			}
			r := resp.Data
			rows = append(rows, testRow{
				Alias:        aliasOrNamespace(reg.Alias, reg.Namespace),
				RegistryType: r.RegistryType,
				LoginOK:      r.LoginOK,
				LoginMessage: r.LoginMessage,
				ManageOK:     r.ManageOK,
				ManageMsg:    r.ManageMessage,
				ManageSkip:   r.ManageSkipped,
			})
		}

		allOK := true
		if err := emitOrPrint(rows, func() {
			for _, r := range rows {
				regType := r.RegistryType
				if regType == "" {
					regType = "acr"
				}
				fmt.Printf("%s [%s]\n", r.Alias, regType)
				if r.LoginOK {
					fmt.Printf("  %s 登录凭证: %s\n", paint(colorGreen, "✓"), r.LoginMessage)
				} else {
					allOK = false
					fmt.Printf("  %s 登录凭证: %s\n", paint(colorRed, "✗"), r.LoginMessage)
				}
				if r.ManageOK || r.ManageMsg != "" {
					switch {
					case r.ManageOK:
						fmt.Printf("  %s 管理面凭证: %s\n", paint(colorGreen, "✓"), r.ManageMsg)
					case r.ManageSkip:
						fmt.Printf("  %s 管理面凭证: %s\n", paint(colorYellow, "–"), r.ManageMsg)
					default:
						fmt.Printf("  %s 管理面凭证: %s\n", paint(colorRed, "✗"), r.ManageMsg)
					}
				}
			}
		}); err != nil {
			return err
		}
		if !allOK {
			return fmt.Errorf("存在不可用的仓库配置，详见上方输出")
		}
		return nil
	},
}

// repoImportCmd dsync repo import --acr 别名：从远程仓库拉取镜像列表导入台账
var repoImportCmd = &cobra.Command{
	Use:   "import --acr <别名>",
	Short: "从远程仓库导入镜像列表到台账（_catalog / 管理面 API）",
	Long: `从远程仓库拉取已有镜像仓库名并导入平台台账（Web 端「从仓库导入」的 CLI 版）：
- ACR / Harbor / 通用 Registry 走 /v2/_catalog；
- 华为云 SWR 走管理面 API（需在仓库配置中填写 AK/SK）；
- 腾讯云 CCR 走腾讯云 API（需 SecretId/SecretKey）。
已存在的仓库自动跳过。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias, _ := cmd.Flags().GetString("acr")
		if alias == "" {
			return fmt.Errorf("必须用 --acr 指定目标仓库（别名优先，dsync acr list 查看）")
		}
		client := newClient()
		reg, err := resolveAcrByNamespace(client, alias)
		if err != nil {
			return err
		}

		var resp importFromRegistryResponse
		if err := client.do("POST", "/acr-repositories/import-from-registry",
			map[string]any{"acr_registry_id": reg.ID}, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp.Data, func() {
			fmt.Printf("导入完成: 新增 %d 个", resp.Data.Created)
			if resp.Data.AlreadyExist > 0 {
				fmt.Printf("，已存在跳过 %d 个", resp.Data.AlreadyExist)
			}
			fmt.Println()
			for _, name := range resp.Data.CreatedNames {
				fmt.Printf("  + %s\n", name)
			}
		})
	},
}

// repoSyncRecordsCmd dsync repo sync-records --acr 别名：从同步记录导入台账
var repoSyncRecordsCmd = &cobra.Command{
	Use:   "sync-records --acr <别名>",
	Short: "从平台同步记录导入镜像台账（本地库对账）",
	Long: `从平台已有的成功同步记录中提取仓库名，校验远程仍存在后导入台账。
适用于台账缺失/漂移时的补齐（与 repo clean 配合完成对账）。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias, _ := cmd.Flags().GetString("acr")
		if alias == "" {
			return fmt.Errorf("必须用 --acr 指定目标仓库（别名优先，dsync acr list 查看）")
		}
		client := newClient()
		reg, err := resolveAcrByNamespace(client, alias)
		if err != nil {
			return err
		}

		var resp syncFromRecordsResponse
		if err := client.do("POST", "/acr-repositories/sync-from-records",
			map[string]any{"acr_registry_id": reg.ID}, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp.Data, func() {
			fmt.Printf("导入完成: 新增 %d 个", resp.Data.Created)
			if resp.Data.AlreadyExist > 0 {
				fmt.Printf("，已存在 %d 个", resp.Data.AlreadyExist)
			}
			if resp.Data.Skipped > 0 {
				fmt.Printf("，跳过 %d 个（远程不存在或校验失败）", resp.Data.Skipped)
			}
			fmt.Println()
			for _, name := range resp.Data.CreatedNames {
				fmt.Printf("  + %s\n", name)
			}
		})
	},
}

// repoCleanCmd dsync repo clean --acr 别名 --yes：清理台账中远程已不存在的记录
var repoCleanCmd = &cobra.Command{
	Use:   "clean --acr <别名> --yes",
	Short: "清理台账中远程仓库已不存在的记录（不删除远程镜像）",
	Long: `逐个校验台账记录在远程仓库的存在性（以 Tag 列表为准），
删除远程已不存在的台账记录。只动平台台账，不删除远程仓库的实际镜像。
需要 --yes 显式确认。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias, _ := cmd.Flags().GetString("acr")
		yes, _ := cmd.Flags().GetBool("yes")
		if alias == "" {
			return fmt.Errorf("必须用 --acr 指定目标仓库（别名优先，dsync acr list 查看）")
		}
		if !yes {
			return fmt.Errorf("清理会删除台账记录，请加 --yes 确认（不影响远程仓库）")
		}
		client := newClient()
		reg, err := resolveAcrByNamespace(client, alias)
		if err != nil {
			return err
		}

		var resp cleanInvalidResponse
		if err := client.do("POST", "/acr-repositories/clean-invalid",
			map[string]any{"acr_registry_id": reg.ID}, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp.Data, func() {
			fmt.Printf("清理完成: 删除 %d 条无效记录", resp.Data.Cleaned)
			if len(resp.Data.CheckFailedNames) > 0 {
				fmt.Printf("，%d 条校验失败保留", len(resp.Data.CheckFailedNames))
			}
			fmt.Println()
			for _, name := range resp.Data.CleanedNames {
				fmt.Printf("  - %s\n", name)
			}
		})
	},
}

func init() {
	acrCmd.AddCommand(acrTestCmd)

	repoImportCmd.Flags().String("acr", "", "目标镜像仓库（别名优先，必填）")
	repoSyncRecordsCmd.Flags().String("acr", "", "目标镜像仓库（别名优先，必填）")
	repoCleanCmd.Flags().String("acr", "", "目标镜像仓库（别名优先，必填）")
	repoCleanCmd.Flags().Bool("yes", false, "确认执行清理")
	repoCmd.AddCommand(repoImportCmd, repoSyncRecordsCmd, repoCleanCmd)
}
