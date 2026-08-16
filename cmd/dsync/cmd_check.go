package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// checkCmd dsync check <image>... [--acr ns]
var checkCmd = &cobra.Command{
	Use:   "check <镜像>... [--acr ns]",
	Short: "检查镜像与 ACR 的归属冲突",
	Long: `检查镜像仓库的 ACR 归属情况：是否已有归属、与所选 ACR 是否冲突。
dsync sync 提交前会自动执行相同检查。`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ns, _ := cmd.Flags().GetString("acr")
		client := newClient()

		req := map[string]any{"images": args}
		if ns != "" {
			reg, err := resolveAcrByNamespace(client, ns)
			if err != nil {
				return err
			}
			req["acr_registry_id"] = reg.ID
		}

		var resp checkAcrResponse
		if err := client.do("POST", "/sync/check-acr", req, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp.Data, func() {
			for _, item := range resp.Data.Items {
				fmt.Printf("%s\n  仓库: %s\n", item.Image, item.RepositoryName)
				switch {
				case item.HasConflict:
					fmt.Printf("  %s\n", paint(colorRed, "冲突: "+item.Message))
				case item.HasAffinity:
					fmt.Printf("  %s 归属 ACR「%s」\n", paint(colorGreen, "已归属"), item.SuggestedNamespace)
				default:
					fmt.Printf("  %s（将写入默认或所选 ACR）\n", paint(colorBlue, "新仓库"))
				}
			}
			if resp.Data.MultiAcrWarning {
				fmt.Println(paint(colorYellow, "注意: 多个镜像归属不同 ACR，提交时将分散到各自归属的 ACR"))
			}
		})
	},
}

// suggestCmd dsync suggest <image>
var suggestCmd = &cobra.Command{
	Use:   "suggest <镜像>",
	Short: "查询镜像推荐同步到哪个 ACR",
	Long: `按平台亲和性逻辑（仓库已有归属 > 默认 ACR > 未满额 ACR）给出推荐结果与理由，
并展示各 ACR 配额用量。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := suggestAcr(newClient(), args[0])
		if err != nil {
			return err
		}

		return emitOrPrint(data, func() {
			fmt.Printf("镜像: %s\n", args[0])
			fmt.Printf("推荐 ACR: %s（%s）\n", data.SuggestedNamespace, suggestReasonText(data.SuggestionReason))
			if data.Affinity.HasAffinity {
				fmt.Printf("归属来源: %s（%s）\n", data.Affinity.AcrNamespace, data.Affinity.Source)
			}
			if len(data.QuotaSummary) > 0 {
				fmt.Println("配额用量:")
				table := make([][]string, 0, len(data.QuotaSummary))
				for _, q := range data.QuotaSummary {
					table = append(table, []string{
						q.Namespace,
						fmt.Sprintf("%d/%d", q.RepoCount, q.RepoQuota),
						fmt.Sprintf("%d", q.RemainingQuota),
						fmt.Sprintf("%v", q.IsFull),
					})
				}
				printTable([]string{"NAMESPACE", "仓库用量", "剩余", "已满"}, table)
			}
		})
	},
}

func init() {
	checkCmd.Flags().String("acr", "", "对比的 ACR（namespace；不指定则只查归属不判冲突）")
	rootCmd.AddCommand(checkCmd, suggestCmd)
}
