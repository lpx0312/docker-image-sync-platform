package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// searchCmd dsync search <关键词> [--acr ns] [--refresh]
var searchCmd = &cobra.Command{
	Use:   "search <关键词>",
	Short: "跨仓库搜索镜像仓库与 Tag",
	Long: `在所有 ACR（或 --acr 指定的 ACR）中按关键词子串搜索仓库名与 Tag。

仓库匹配来自平台本地库（即时）；Tag 匹配来自本地 tag 缓存，
缓存缺失或超过 24 小时会自动增量拉取（可 --refresh 强制全量刷新）。
首次搜索会拉取全部仓库的 Tag，视仓库数量可能耗时几十秒。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword := args[0]
		force, _ := cmd.Flags().GetBool("refresh")
		client := newClient()

		scope, err := scopeRegistries(cmd)
		if err != nil {
			return err
		}

		// 1. 仓库名匹配（平台本地库，即时）
		type repoHit struct {
			Namespace string `json:"namespace"`
			Repo      string `json:"repository"`
		}
		var repoHits []repoHit
		scopeIDs := map[uint]bool{}
		for _, reg := range scope {
			scopeIDs[reg.ID] = true
			repos, err := fetchRepositories(client, reg.ID)
			if err != nil {
				return fmt.Errorf("获取 %s 的仓库列表失败: %w", reg.Namespace, err)
			}
			for _, r := range repos {
				if strings.Contains(r.RepositoryName, keyword) {
					repoHits = append(repoHits, repoHit{Namespace: reg.Namespace, Repo: r.RepositoryName})
				}
			}
		}

		// 2. Tag 匹配（本地缓存，必要时增量刷新）
		if _, failed, err := ensureTagCacheFresh(client, scope, force); err != nil {
			return err
		} else if failed > 0 {
			fmt.Fprintf(os.Stderr, "警告: %d 个仓库的 Tag 拉取失败，结果可能不完整\n", failed)
		}
		cache := loadTagCache()

		type tagHit struct {
			Namespace string `json:"namespace"`
			Repo      string `json:"repository"`
			Tag       string `json:"tag"`
		}
		var tagHits []tagHit
		for _, entry := range cache.Entries {
			if !scopeIDs[entry.AcrRegistryID] {
				continue
			}
			for _, tag := range entry.Tags {
				if strings.Contains(tag, keyword) {
					tagHits = append(tagHits, tagHit{
						Namespace: entry.Namespace,
						Repo:      entry.RepositoryName,
						Tag:       tag,
					})
				}
			}
		}
		sort.Slice(tagHits, func(i, j int) bool {
			if tagHits[i].Namespace != tagHits[j].Namespace {
				return tagHits[i].Namespace < tagHits[j].Namespace
			}
			if tagHits[i].Repo != tagHits[j].Repo {
				return tagHits[i].Repo < tagHits[j].Repo
			}
			return tagHits[i].Tag < tagHits[j].Tag
		})

		result := struct {
			Keyword string    `json:"keyword"`
			Repos   []repoHit `json:"repo_matches"`
			Tags    []tagHit  `json:"tag_matches"`
		}{Keyword: keyword, Repos: repoHits, Tags: tagHits}
		if result.Repos == nil {
			result.Repos = []repoHit{}
		}
		if result.Tags == nil {
			result.Tags = []tagHit{}
		}

		return emitOrPrint(result, func() {
			if len(repoHits) == 0 && len(tagHits) == 0 {
				fmt.Printf("未找到与 %q 匹配的仓库或 Tag\n", keyword)
				return
			}
			fmt.Printf("仓库匹配 %d 个:\n", len(repoHits))
			table := make([][]string, 0, len(repoHits))
			for _, h := range repoHits {
				table = append(table, []string{h.Namespace, h.Repo})
			}
			printTable([]string{"ACR(NAMESPACE)", "仓库"}, table)

			const displayLimit = 200
			fmt.Printf("\nTag 匹配 %d 个:\n", len(tagHits))
			tagTable := make([][]string, 0, len(tagHits))
			for _, h := range tagHits {
				if len(tagTable) >= displayLimit {
					break
				}
				tagTable = append(tagTable, []string{h.Namespace, h.Repo, h.Tag})
			}
			printTable([]string{"ACR(NAMESPACE)", "仓库", "TAG"}, tagTable)
			if len(tagHits) > displayLimit {
				fmt.Printf("（仅显示前 %d 条，完整结果请用 --json）\n", displayLimit)
			}
		})
	},
}

func init() {
	searchCmd.Flags().String("acr", "", "限定 ACR（namespace，默认全部）")
	searchCmd.Flags().Bool("refresh", false, "强制全量刷新 tag 缓存")
	rootCmd.AddCommand(searchCmd)
}
