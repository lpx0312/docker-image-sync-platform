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
	Short: "镜像仓库查询（数据源：平台本地库）",
}

// repoListCmd dsync repo list [--acr ns] [--filter kw]
var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看镜像仓库列表",
	Long: `查看平台本地库中的镜像仓库列表。
--acr 未指定时遍历所有 ACR。数据来自平台本地库，
如与 ACR 实际状态不一致，可在 Web 端执行"从同步记录导入/清理无效镜像"对账。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, _ := cmd.Flags().GetString("filter")
		regs, err := scopeRegistries(cmd)
		if err != nil {
			return err
		}
		client := newClient()

		type repoRow struct {
			Namespace string `json:"namespace"`
			Repo      string `json:"repository"`
			CreatedAt string `json:"created_at"`
		}
		var rows []repoRow
		for _, reg := range regs {
			repos, err := fetchRepositories(client, reg.ID)
			if err != nil {
				return fmt.Errorf("获取 %s 的仓库列表失败: %w", reg.Namespace, err)
			}
			for _, r := range repos {
				if filter != "" && !strings.Contains(r.RepositoryName, filter) {
					continue
				}
				rows = append(rows, repoRow{
					Namespace: reg.Namespace,
					Repo:      r.RepositoryName,
					CreatedAt: r.CreatedAt.Format("2006-01-02 15:04"),
				})
			}
		}

		return emitOrPrint(rows, func() {
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Namespace, r.Repo, r.CreatedAt})
			}
			printTable([]string{"ACR(NAMESPACE)", "仓库", "录入时间"}, table)
			fmt.Printf("共 %d 个仓库\n", len(rows))
		})
	},
}

func init() {
	repoListCmd.Flags().String("acr", "", "限定 ACR（namespace，默认全部）")
	repoListCmd.Flags().String("filter", "", "按仓库名子串过滤")
	repoCmd.AddCommand(repoListCmd)
	rootCmd.AddCommand(repoCmd)
}
