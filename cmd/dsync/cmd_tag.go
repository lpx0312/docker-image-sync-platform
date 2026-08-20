package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// tagCmd dsync tag 命令组
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "镜像 Tag 查询（实时查目标仓库）",
}

// tagListCmd dsync tag list <repo> [--acr ns]
var tagListCmd = &cobra.Command{
	Use:   "list <repo>",
	Short: "查看某仓库的全部 Tag（实时）",
	Long: `实时查询指定仓库的全部 Tag（按目标仓库类型自动适配认证）。
--acr 未指定时先在所有仓库的本地库中定位仓库：
唯一命中直接查询；多处命中则全部展示；未命中则报错。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := args[0]
		ns, _ := cmd.Flags().GetString("acr")
		client := newClient()

		var targets []AcrRegistryInfo
		if ns != "" {
			reg, err := resolveAcrByNamespace(client, ns)
			if err != nil {
				return err
			}
			targets = append(targets, *reg)
		} else {
			regs, err := listRegistries(client)
			if err != nil {
				return err
			}
			for _, reg := range regs {
				repos, err := fetchRepositories(client, reg.ID)
				if err != nil {
					return fmt.Errorf("获取 %s 的仓库列表失败: %w", reg.Namespace, err)
				}
				for _, r := range repos {
					if r.RepositoryName == repo {
						targets = append(targets, reg)
						break
					}
				}
			}
			if len(targets) == 0 {
				return fmt.Errorf("仓库 %q 在平台本地库中不存在；若确认远程仓库中已存在，请用 --acr 指定所属仓库（别名优先）", repo)
			}
		}

		type tagRow struct {
			Namespace string   `json:"namespace"`
			Repo      string   `json:"repository"`
			Tags      []string `json:"tags"`
		}
		rows := make([]tagRow, 0, len(targets))
		for _, reg := range targets {
			tags, err := fetchTagNames(client, reg.ID, repo)
			if err != nil {
				return fmt.Errorf("查询 %s/%s 的 Tag 失败: %w", reg.Namespace, repo, err)
			}
			rows = append(rows, tagRow{Namespace: reg.Namespace, Repo: repo, Tags: tags})
		}

		return emitOrPrint(rows, func() {
			for _, r := range rows {
				fmt.Printf("%s/%s（%d 个 Tag）:\n", r.Namespace, r.Repo, len(r.Tags))
				// 每行 6 个，避免长列表刷屏
				for i := 0; i < len(r.Tags); i += 6 {
					end := i + 6
					if end > len(r.Tags) {
						end = len(r.Tags)
					}
					fmt.Println("  " + strings.Join(r.Tags[i:end], "  "))
				}
			}
		})
	},
}

// tagDetailCmd dsync tag detail <repo> <tag> [--acr ns]：查看单个 Tag 详情
var tagDetailCmd = &cobra.Command{
	Use:   "detail <repo> <tag>",
	Short: "查看某 Tag 的详细信息（架构/digest/大小/推送时间）",
	Long: `实时查询指定仓库中某个 Tag 的详细信息（manifest 摘要、架构、digest、大小、推送时间）。
--acr 未指定时先在所有仓库的本地库中定位仓库：
唯一命中直接查询；多处命中则全部展示；未命中则报错。`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, tag := args[0], args[1]
		ns, _ := cmd.Flags().GetString("acr")
		client := newClient()

		var targets []AcrRegistryInfo
		if ns != "" {
			reg, err := resolveAcrByNamespace(client, ns)
			if err != nil {
				return err
			}
			targets = append(targets, *reg)
		} else {
			regs, err := listRegistries(client)
			if err != nil {
				return err
			}
			for _, reg := range regs {
				repos, err := fetchRepositories(client, reg.ID)
				if err != nil {
					return fmt.Errorf("获取 %s 的仓库列表失败: %w", reg.Namespace, err)
				}
				for _, r := range repos {
					if r.RepositoryName == repo {
						targets = append(targets, reg)
						break
					}
				}
			}
			if len(targets) == 0 {
				return fmt.Errorf("仓库 %q 在平台本地库中不存在；若确认远程仓库中已存在，请用 --acr 指定所属仓库（别名优先）", repo)
			}
		}

		type detailRow struct {
			Alias string      `json:"alias"`
			Repo  string      `json:"repository"`
			Tag   string      `json:"tag"`
			Detail tagDetailData `json:"detail"`
		}
		rows := make([]detailRow, 0, len(targets))
		for _, reg := range targets {
			var resp tagDetailResponse
			path := fmt.Sprintf("/acr-tags/detail?acr_registry_id=%d&repository_name=%s&tag=%s",
				reg.ID, url.QueryEscape(repo), url.QueryEscape(tag))
			if err := client.do("GET", path, nil, &resp); err != nil {
				return fmt.Errorf("查询 %s/%s:%s 失败: %w", reg.Namespace, repo, tag, err)
			}
			rows = append(rows, detailRow{
				Alias:  aliasOrNamespace(reg.Alias, reg.Namespace),
				Repo:   repo,
				Tag:    tag,
				Detail: resp.Data,
			})
		}

		return emitOrPrint(rows, func() {
			for _, r := range rows {
				fmt.Printf("%s/%s:%s\n", r.Alias, r.Repo, r.Tag)
				d := r.Detail
				if len(d.Architectures) == 0 {
					fmt.Printf("  %s（无架构信息，可能 Tag 不存在或 manifest 不可读）\n", paint(colorYellow, "–"))
					continue
				}
				fmt.Printf("  架构: %s\n", strings.Join(d.Architectures, ", "))
				for _, arch := range d.Architectures {
					digest := d.Digests[arch]
					size := d.Sizes[arch]
					pushed := d.PushedAt[arch]
					sizeStr := "-"
					if size > 0 {
						sizeStr = humanBytes(size)
					}
					if pushed == "" {
						pushed = "-"
					}
					fmt.Printf("    %s  %s  %s  %s\n", arch, shortDigest(digest), sizeStr, pushed)
				}
			}
		})
	},
}

func init() {
	tagListCmd.Flags().String("acr", "", "仓库所属镜像仓库（别名优先，默认自动定位）")
	tagDetailCmd.Flags().String("acr", "", "仓库所属镜像仓库（别名优先，默认自动定位）")
	tagCmd.AddCommand(tagListCmd, tagDetailCmd)
	rootCmd.AddCommand(tagCmd)
}
