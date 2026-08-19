package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// scopeRegistries 根据 --acr 参数确定命令作用域：
// 指定了 namespace 则只取该 ACR，否则取全部。
func scopeRegistries(cmd *cobra.Command) ([]AcrRegistryInfo, error) {
	ns, _ := cmd.Flags().GetString("acr")
	if ns == "" {
		return listRegistries(newClient())
	}
	reg, err := resolveAcrByNamespace(newClient(), ns)
	if err != nil {
		return nil, err
	}
	return []AcrRegistryInfo{*reg}, nil
}

// repoCmd dsync repo 命令组
var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "镜像仓库台账查询与对账（数据源：平台本地库）",
}

// repoListCmd dsync repo list [--acr ns] [--filter kw]
var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看镜像仓库列表",
	Long: `查看平台台账中的镜像仓库列表。
--acr 未指定时遍历所有仓库（别名优先，兼容 namespace）。
数据来自平台本地库，可能与远程实际状态漂移，
可用 repo import / repo sync-records / repo clean 完成对账。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, _ := cmd.Flags().GetString("filter")
		regs, err := scopeRegistries(cmd)
		if err != nil {
			return err
		}
		client := newClient()

		type repoRow struct {
			Alias     string `json:"alias"`
			Repo      string `json:"repository"`
			CreatedAt string `json:"created_at"`
		}
		var rows []repoRow
		for _, reg := range regs {
			repos, err := fetchRepositories(client, reg.ID)
			if err != nil {
				return fmt.Errorf("获取 %s 的仓库列表失败: %w", aliasOrNamespace(reg.Alias, reg.Namespace), err)
			}
			for _, r := range repos {
				if filter != "" && !strings.Contains(r.RepositoryName, filter) {
					continue
				}
				rows = append(rows, repoRow{
					Alias:     aliasOrNamespace(reg.Alias, reg.Namespace),
					Repo:      r.RepositoryName,
					CreatedAt: r.CreatedAt.Format("2006-01-02 15:04"),
				})
			}
		}

		return emitOrPrint(rows, func() {
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Alias, r.Repo, r.CreatedAt})
			}
			printTable([]string{"ALIAS", "仓库", "录入时间"}, table)
			fmt.Printf("共 %d 个仓库\n", len(rows))
		})
	},
}

func init() {
	repoListCmd.Flags().String("acr", "", "限定镜像仓库（别名优先，默认全部）")
	repoListCmd.Flags().String("filter", "", "按仓库名子串过滤")
	repoCmd.AddCommand(repoListCmd)
	rootCmd.AddCommand(repoCmd)
}
