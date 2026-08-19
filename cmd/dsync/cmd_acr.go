package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// acrCmd dsync acr 命令组
var acrCmd = &cobra.Command{
	Use:   "acr",
	Short: "镜像仓库实例管理（阿里云 ACR / 华为云 SWR）",
}

// acrListCmd dsync acr list：仓库列表 + 配额用量
var acrListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看所有镜像仓库及配额用量",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		regs, err := listRegistries(client)
		if err != nil {
			return err
		}
		quotas, err := fetchQuotaSummary(client)
		if err != nil {
			return err
		}
		quotaByAcr := make(map[uint]quotaSummaryItem, len(quotas))
		for _, q := range quotas {
			quotaByAcr[q.AcrRegistryID] = q
		}

		type row struct {
			ID           uint              `json:"id"`
			Namespace    string            `json:"namespace"`
			Registry     string            `json:"registry_url"`
			RegistryType string            `json:"registry_type"`
			IsDefault    bool              `json:"is_default"`
			RepoCount    int               `json:"repo_count"`
			RepoQuota    int               `json:"repo_quota"`
			Remaining    int               `json:"remaining_quota"`
			IsFull       bool              `json:"is_full"`
			Extra        map[string]string `json:"extra,omitempty"`
		}
		rows := make([]row, 0, len(regs))
		for _, r := range regs {
			item := row{
				ID:           r.ID,
				Namespace:    r.Namespace,
				Registry:     r.RegistryURL,
				RegistryType: r.RegistryType,
				IsDefault:    r.IsDefault,
			}
			if q, ok := quotaByAcr[r.ID]; ok {
				item.RepoCount = q.RepoCount
				item.RepoQuota = q.RepoQuota
				item.Remaining = q.RemainingQuota
				item.IsFull = q.IsFull
			}
			rows = append(rows, item)
		}

		return emitOrPrint(rows, func() {
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				defMark := ""
				if r.IsDefault {
					defMark = "*"
				}
				registryType := r.RegistryType
				if registryType == "" {
					registryType = "acr"
				}
				usage := fmt.Sprintf("%d/%d", r.RepoCount, r.RepoQuota)
				if r.RepoQuota == 0 {
					usage = fmt.Sprintf("%d/不限", r.RepoCount)
				}
				if r.IsFull {
					usage = paint(colorRed, usage+" (满)")
				}
				remaining := fmt.Sprintf("%d", r.Remaining)
				if r.RepoQuota == 0 {
					remaining = "-"
				}
				table = append(table, []string{
					fmt.Sprintf("%d%s", r.ID, defMark),
					r.Namespace,
					registryType,
					r.Registry,
					usage,
					remaining,
				})
			}
			printTable([]string{"ID", "NAMESPACE", "TYPE", "REGISTRY", "仓库用量", "剩余配额"}, table)
			fmt.Println("(* 为默认仓库；引用仓库时使用 NAMESPACE 列的值，如 --acr <namespace>)")
		})
	},
}

func init() {
	acrCmd.AddCommand(acrListCmd)
	rootCmd.AddCommand(acrCmd)
}
