package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// parseBatchFile 解析批量同步文件。
// 每行格式: 源镜像[:tag][ 空格 目标Tag]，支持 # 注释与空行。
func parseBatchFile(path string) ([]batchImageItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	var items []batchImageItem
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		item := batchImageItem{SourceImage: fields[0]}
		if len(fields) >= 2 {
			item.TargetTag = fields[1]
		}
		if item.SourceImage == "" {
			return nil, fmt.Errorf("第 %d 行镜像地址为空", lineNo)
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("文件中没有有效镜像")
	}
	return items, nil
}

// batchCmd dsync batch -f images.txt
var batchCmd = &cobra.Command{
	Use:   "batch -f <文件>",
	Short: "批量提交镜像同步任务",
	Long: `从文件批量提交镜像同步任务，默认阻塞等待并在完成后打印目标镜像地址。

文件格式（每行一个镜像）:
  nginx:1.25
  redis:7-alpine  7-alpine-amd64      # 源镜像 + 目标Tag（可选）
  # 井号开头为注释

注意: 批量同步多于 1 个镜像时，服务端会按仓库亲和性自动分配 ACR，
--acr 仅作为无归属仓库的首选，并非强制指定。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return fmt.Errorf("请通过 -f 指定镜像列表文件")
		}
		ns, _ := cmd.Flags().GetString("acr")
		arch, _ := cmd.Flags().GetString("arch")
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		autoRetry, _ := cmd.Flags().GetBool("auto-retry")
		retryCount, _ := cmd.Flags().GetInt("retry-count")
		noWait, _ := cmd.Flags().GetBool("no-wait")

		items, err := parseBatchFile(file)
		if err != nil {
			return err
		}

		req := batchRequest{
			Images:        items,
			MaxConcurrent: concurrency,
			AutoRetry:     autoRetry,
			AcrRegistryID: 0,
		}
		if arch != "" {
			for i := range req.Images {
				req.Images[i].Architecture = arch
			}
		}
		if autoRetry && retryCount > 0 {
			req.RetryCount = retryCount
		}
		if ns != "" {
			reg, err := resolveAcrByNamespace(newClient(), ns)
			if err != nil {
				return err
			}
			req.AcrRegistryID = reg.ID
		}

		var submitted submitResponse
		client := newClient()
		if err := client.postSyncLimited("/sync/batch", req, &submitted); err != nil {
			return err
		}

		if noWait {
			return emitOrPrint(submitted, func() {
				fmt.Printf("批量任务已提交: %s（%d 个镜像）\n", submitted.TaskID, submitted.TotalImages)
				fmt.Printf("查看进度: dsync task status %s\n", submitted.TaskID)
			})
		}

		status, err := waitForTask(client, submitted.TaskID, true)
		if err != nil {
			return err
		}
		return emitOrPrint(status, func() {
			printTaskResult(status)
		})
	},
}

func init() {
	batchCmd.Flags().StringP("file", "f", "", "镜像列表文件路径（每行一个镜像）")
	batchCmd.Flags().String("acr", "", "首选 ACR（namespace；多镜像时服务端仍按亲和性分配）")
	batchCmd.Flags().String("arch", "", "目标架构，如 amd64 / arm64")
	batchCmd.Flags().Int("concurrency", 0, "最大并发数（0 表示使用服务端配置）")
	batchCmd.Flags().Bool("auto-retry", false, "失败自动重试")
	batchCmd.Flags().Int("retry-count", 0, "自动重试次数（0 表示使用服务端配置）")
	batchCmd.Flags().Bool("no-wait", false, "提交后立即返回 task-id，不等待完成")
	rootCmd.AddCommand(batchCmd)
}
