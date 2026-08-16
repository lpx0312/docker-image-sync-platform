package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// taskCmd dsync task 命令组
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "同步任务查询",
}

// taskListCmd dsync task list
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看同步任务历史",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, _ := cmd.Flags().GetString("status")
		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		long, _ := cmd.Flags().GetBool("long")

		path := fmt.Sprintf("/sync/history?page=%d&page_size=%d", page, pageSize)
		if status != "" {
			path += "&status=" + status
		}
		var resp historyResponse
		if err := newClient().do("GET", path, nil, &resp); err != nil {
			return err
		}

		return emitOrPrint(resp, func() {
			table := make([][]string, 0, len(resp.Data))
			for _, t := range resp.Data {
				id := shortID(t.TaskID)
				if long {
					id = t.TaskID
				}
				images := fmt.Sprintf("%d/%d", t.CompletedImages, t.TotalImages)
				if t.FailedImages > 0 {
					images += fmt.Sprintf(" 失败%d", t.FailedImages)
				}
				row := []string{
					id,
					colorStatus(t.Status),
					images,
					fmt.Sprintf("%.0f%%", t.Progress),
					t.CreatedAt.Format("01-02 15:04"),
				}
				if long && t.GitHubActionURL != "" {
					row = append(row, t.GitHubActionURL)
				}
				table = append(table, row)
			}
			headers := []string{"TASK", "状态", "镜像(完成/总数)", "进度", "创建时间"}
			if long {
				headers = append(headers, "ACTIONS")
			}
			printTable(headers, table)
			fmt.Printf("共 %d 个任务（第 %d 页，每页 %d）\n", resp.Total, resp.Page, resp.PageSize)
		})
	},
}

// taskStatusCmd dsync task status <task-id> [--watch]
var taskStatusCmd = &cobra.Command{
	Use:   "status <task-id>",
	Short: "查看任务状态详情",
	Long: `查看指定同步任务的状态详情。
默认单次查询；--watch 持续轮询直至终态（同 dsync sync 的等待行为）。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		watch, _ := cmd.Flags().GetBool("watch")
		client := newClient()
		st, err := waitForTask(client, args[0], watch)
		if err != nil {
			return err
		}
		return emitOrPrint(st, func() {
			printTaskResult(st)
		})
	},
}

// retryCmd dsync retry <record-id> / dsync retry --task <task-id>
var retryCmd = &cobra.Command{
	Use:   "retry <record-id> | --task <task-id>",
	Short: "重试失败的镜像同步记录",
	Long: `重试失败的镜像同步记录。
可直接指定记录 ID，或用 --task <task-id> 重试该任务下全部失败记录。
记录 ID 可通过 dsync task status <task-id> 查看。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, _ := cmd.Flags().GetString("task")
		client := newClient()

		var recordIDs []string
		if taskID != "" {
			st, err := getSyncStatus(client, taskID)
			if err != nil {
				return err
			}
			for _, r := range st.Images.Records {
				if r.SyncStatus == "failed" {
					recordIDs = append(recordIDs, strconv.FormatUint(uint64(r.ID), 10))
				}
			}
			if len(recordIDs) == 0 {
				fmt.Println("该任务没有失败记录，无需重试")
				return nil
			}
		} else {
			if len(args) != 1 {
				return fmt.Errorf("请指定记录 ID，或用 --task <task-id> 重试任务下全部失败记录")
			}
			recordIDs = []string{args[0]}
		}

		for _, id := range recordIDs {
			var resp map[string]any
			if err := client.do("POST", "/images/"+id+"/retry", nil, &resp); err != nil {
				return fmt.Errorf("重试记录 %s 失败: %w", id, err)
			}
			fmt.Printf("记录 %s 已重置为待同步\n", id)
		}
		fmt.Println("提示: 重置后的记录将由平台重新调度，可用 dsync task list --status running 跟踪")
		return nil
	},
}

func init() {
	taskListCmd.Flags().String("status", "", "按状态过滤（pending/running/completed/failed/partial_success）")
	taskListCmd.Flags().Int("page", 1, "页码")
	taskListCmd.Flags().Int("page-size", 20, "每页数量")
	taskListCmd.Flags().Bool("long", false, "显示完整 task-id 与 Actions 链接")
	taskStatusCmd.Flags().Bool("watch", false, "持续轮询直至终态")
	retryCmd.Flags().String("task", "", "重试该任务下全部失败记录")

	taskCmd.AddCommand(taskListCmd, taskStatusCmd)
	rootCmd.AddCommand(taskCmd, retryCmd)
}
