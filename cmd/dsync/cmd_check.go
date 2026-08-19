package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// checkCmd dsync check <image>... [--acr ns]
var checkCmd = &cobra.Command{
	Use:   "check <镜像>... [--acr ns]",
	Short: "检查镜像与仓库的归属冲突",
	Long: `检查镜像的仓库归属情况：是否已有归属、与所选仓库是否冲突。
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
					fmt.Printf("  %s 归属仓库「%s」\n", paint(colorGreen, "已归属"), aliasOrNamespace(item.SuggestedAlias, item.SuggestedNamespace))
				default:
					fmt.Printf("  %s（将写入默认或所选 ACR）\n", paint(colorBlue, "新仓库"))
				}
			}
			if resp.Data.MultiAcrWarning {
				fmt.Println(paint(colorYellow, "注意: 多个镜像归属不同仓库，提交时将分散到各自归属的仓库"))
			}
		})
	},
}

// suggestCmd dsync suggest <image>
var suggestCmd = &cobra.Command{
	Use:   "suggest <镜像>",
	Short: "查询镜像推荐同步到哪个仓库",
	Long: `按平台亲和性逻辑（仓库已有归属 > 默认仓库 > 未满额仓库）给出推荐结果与理由，
并展示各仓库配额用量。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := suggestAcr(newClient(), args[0])
		if err != nil {
			return err
		}

		return emitOrPrint(data, func() {
			fmt.Printf("镜像: %s\n", args[0])
			fmt.Printf("推荐仓库: %s（%s）\n", aliasOrNamespace(data.SuggestedAlias, data.SuggestedNamespace), suggestReasonText(data.SuggestionReason))
			if data.Affinity.HasAffinity {
				fmt.Printf("归属来源: %s（%s）\n", aliasOrNamespace(data.Affinity.AcrAlias, data.Affinity.AcrNamespace), data.Affinity.Source)
			}
			if len(data.QuotaSummary) > 0 {
				fmt.Println("配额用量:")
				table := make([][]string, 0, len(data.QuotaSummary))
				for _, q := range data.QuotaSummary {
					usage := fmt.Sprintf("%d/%d", q.RepoCount, q.RepoQuota)
					remaining := fmt.Sprintf("%d", q.RemainingQuota)
					if q.RepoQuota == 0 {
						usage = fmt.Sprintf("%d/不限", q.RepoCount)
						remaining = "-"
					}
					table = append(table, []string{
						aliasOrNamespace(q.Alias, q.Namespace),
						usage,
						remaining,
						fmt.Sprintf("%v", q.IsFull),
					})
				}
				printTable([]string{"ALIAS", "仓库用量", "剩余", "已满"}, table)
			}
		})
	},
}

func init() {
	checkCmd.Flags().String("acr", "", "对比的镜像仓库（别名优先；不指定则只查归属不判冲突）")
	rootCmd.AddCommand(checkCmd, suggestCmd)
}
