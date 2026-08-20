package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// imageCmd dsync image 命令组：镜像同步记录查询与管理
var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "镜像同步记录查询与管理",
}

// imageListCmd dsync image list：镜像同步记录列表（分页/过滤/搜索/排序/去重）
var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看镜像同步记录列表",
	Long: `查看镜像同步记录列表（按单条镜像记录维度，区别于 task list 的任务维度）。
支持按状态、架构过滤与关键词搜索，可去重（相同源镜像/Tag/目标/架构/状态只保留最新一条）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		status, _ := cmd.Flags().GetString("status")
		search, _ := cmd.Flags().GetString("search")
		arch, _ := cmd.Flags().GetString("arch")
		sortBy, _ := cmd.Flags().GetString("sort-by")
		sortOrder, _ := cmd.Flags().GetString("sort-order")
		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		dedup, _ := cmd.Flags().GetBool("deduplicate")
		long, _ := cmd.Flags().GetBool("long")

		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page_size", strconv.Itoa(pageSize))
		if status != "" {
			params.Set("status", status)
		}
		if search != "" {
			params.Set("search", search)
		}
		if arch != "" {
			params.Set("architecture", arch)
		}
		if sortBy != "" {
			params.Set("sort_by", sortBy)
		}
		if sortOrder != "" {
			params.Set("sort_order", sortOrder)
		}
		if dedup {
			params.Set("deduplicate", "true")
		}

		var resp imageListResponse
		if err := newClient().do("GET", "/images/list?"+params.Encode(), nil, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp, func() {
			table := make([][]string, 0, len(resp.Data))
			for _, r := range resp.Data {
				addr := r.ACRImage
				if addr == "" {
					addr = paint(colorGray, "(未回填)")
				}
				row := []string{
					strconv.FormatUint(uint64(r.ID), 10),
					colorStatus(r.SyncStatus),
					r.OriginalImage,
					r.Tag,
					addr,
					r.CreatedAt.Format("01-02 15:04"),
				}
				if long {
					row = append(row, r.TaskID)
				}
				table = append(table, row)
			}
			headers := []string{"ID", "状态", "源镜像", "TAG", "目标地址", "创建时间"}
			if long {
				headers = append(headers, "TASK")
			}
			printTable(headers, table)
			fmt.Printf("共 %d 条记录（第 %d 页，每页 %d）\n", resp.Total, resp.Page, resp.PageSize)
		})
	},
}

// imageStatsCmd dsync image stats：镜像同步状态统计
var imageStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "查看镜像同步状态统计",
	Long:  `统计各同步状态的镜像记录数量（待同步/同步中/成功/失败/总计）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var resp imageStatsResponse
		if err := newClient().do("GET", "/images/stats", nil, &resp); err != nil {
			return err
		}
		return emitOrPrint(resp, func() {
			fmt.Printf("总计: %d\n", resp.Total)
			fmt.Printf("  %s 待同步: %d\n", paint(colorYellow, "·"), resp.Pending)
			fmt.Printf("  %s 同步中: %d\n", paint(colorBlue, "·"), resp.Syncing)
			fmt.Printf("  %s 成功:   %d\n", paint(colorGreen, "·"), resp.Success)
			fmt.Printf("  %s 失败:   %d\n", paint(colorRed, "·"), resp.Failed)
		})
	},
}

// imageDeleteCmd dsync image delete <id>
var imageDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "删除镜像同步记录",
	Long: `删除指定的镜像同步记录（仅删除平台记录，不影响远程仓库中的实际镜像）。
记录 ID 可通过 dsync image list 查看。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			return fmt.Errorf("删除仅移除平台记录、不影响远程镜像；确认请加 --yes")
		}
		var resp map[string]any
		if err := newClient().do("DELETE", "/images/"+args[0], nil, &resp); err != nil {
			return err
		}
		return emitOrPrint(resp, func() {
			fmt.Printf("镜像记录 %s 已删除\n", args[0])
		})
	},
}

// imageCheckCmd dsync image check <id>：校验镜像在远程仓库是否存在并更新状态
var imageCheckCmd = &cobra.Command{
	Use:   "check <id>",
	Short: "校验镜像在远程仓库是否存在",
	Long: `检查指定镜像记录在目标仓库是否真实存在，并按结果自动更新同步状态
（存在则标记成功并刷新架构信息，不存在则标记失败）。用于修复状态与实际不一致。
记录 ID 可通过 dsync image list 查看。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var resp imageCheckResponse
		if err := newClient().do("POST", "/images/"+args[0]+"/check", nil, &resp); err != nil {
			return err
		}
		return emitOrPrint(resp, func() {
			mark := paint(colorRed, "✗")
			if resp.Exists {
				mark = paint(colorGreen, "✓")
			}
			fmt.Printf("%s 镜像 %s\n", mark, resp.TargetImage)
			if len(resp.Architectures) > 0 {
				fmt.Printf("  架构: %s\n", strings.Join(resp.Architectures, ", "))
			}
			fmt.Printf("  %s\n", resp.Message)
		})
	},
}

func init() {
	imageListCmd.Flags().String("status", "", "按状态过滤（pending/syncing/success/failed/retrying/skipped）")
	imageListCmd.Flags().String("search", "", "搜索关键词（匹配源镜像/目标镜像/Tag/描述/任务ID）")
	imageListCmd.Flags().String("arch", "", "按架构过滤（amd64/arm64 等）")
	imageListCmd.Flags().String("sort-by", "created_at", "排序字段（original_image/sync_status/architecture/created_at/updated_at）")
	imageListCmd.Flags().String("sort-order", "desc", "排序方向（asc/desc）")
	imageListCmd.Flags().Int("page", 1, "页码")
	imageListCmd.Flags().Int("page-size", 20, "每页数量（最大 100）")
	imageListCmd.Flags().Bool("deduplicate", false, "去重（相同源镜像/Tag/目标/架构/状态只保留最新一条）")
	imageListCmd.Flags().Bool("long", false, "显示关联的 task-id")

	imageDeleteCmd.Flags().Bool("yes", false, "确认删除")

	imageCmd.AddCommand(imageListCmd, imageStatsCmd, imageDeleteCmd, imageCheckCmd)
	rootCmd.AddCommand(imageCmd)
}
