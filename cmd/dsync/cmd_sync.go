package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// dedupOutcome 查重判定结果
type dedupOutcome struct {
	Action  string // proceed | info | block
	Message string
}

// decideDedup 依据 check-acr 结果与目标 tag 是否已存在，判定能否提交同步。
//   - 仓库已归属其他 ACR → block（避免跨 ACR 重复建仓）
//   - 仓库已存在于目标 ACR 且同名 tag 已存在 → block（纯重复同步，消息含完整可拉取地址）
//   - 仓库已存在但 tag 是新的 → info（追加 tag，允许）
//   - 全新仓库 → proceed
func decideDedup(item checkAcrItem, effectiveAcrID uint, registryURL, effectiveAlias, effectiveNs, targetTag string, existingTags []string, tagsKnown bool) dedupOutcome {
	if item.HasAffinity && item.SuggestedAcrID != effectiveAcrID {
		return dedupOutcome{
			Action: "block",
			Message: fmt.Sprintf("仓库 %s 已归属镜像仓库「%s」，建议同步到那里；确要写入「%s」请加 --force",
				item.RepositoryName, aliasOrNamespace(item.SuggestedAlias, item.SuggestedNamespace), aliasOrNamespace(effectiveAlias, effectiveNs)),
		}
	}
	if item.HasAffinity {
		if tagsKnown {
			for _, t := range existingTags {
				if t == targetTag {
					// 输出带 registry 域名的完整地址，用户可直接 docker pull
					addr := fmt.Sprintf("%s/%s/%s:%s", registryURL, effectiveNs, item.RepositoryName, targetTag)
					return dedupOutcome{
						Action:  "block",
						Message: fmt.Sprintf("镜像已存在于目标 ACR（%s），无需重复同步；确要重试请加 --force", addr),
					}
				}
			}
			return dedupOutcome{
				Action:  "info",
				Message: fmt.Sprintf("仓库 %s 已存在于该 ACR，将追加新 Tag %s", item.RepositoryName, targetTag),
			}
		}
		return dedupOutcome{
			Action:  "info",
			Message: fmt.Sprintf("仓库 %s 已存在于该 ACR（Tag 校验跳过），如需精确查重请稍后重试", item.RepositoryName),
		}
	}
	return dedupOutcome{Action: "proceed"}
}

// syncCmd dsync sync <source-image> [flags]
var syncCmd = &cobra.Command{
	Use:   "sync <源镜像> [flags]",
	Short: "提交镜像同步任务并等待完成",
	Long: `提交镜像同步任务，默认阻塞等待并在完成后打印目标镜像地址。

示例:
  dsync sync nginx:1.25
  dsync sync ghcr.io/prometheus/prometheus:v2.50.0 --acr my-ns
  dsync sync k8s.gcr.io/pause:3.9 --target-tag 3.9-amd64 --arch amd64
  dsync sync nginx:1.25 --no-wait          # 只拿 task-id

提交前自动查重（--force 跳过）：
  - 仓库已归属其他 ACR 时拦截
  - 目标 ACR 已存在相同 仓库:Tag 时拦截
未指定 --acr 时按服务端亲和性逻辑自动选择目标 ACR（与 Web 端一致）。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		image := args[0]
		ns, _ := cmd.Flags().GetString("acr")
		targetTag, _ := cmd.Flags().GetString("target-tag")
		arch, _ := cmd.Flags().GetString("arch")
		desc, _ := cmd.Flags().GetString("desc")
		force, _ := cmd.Flags().GetBool("force")
		noWait, _ := cmd.Flags().GetBool("no-wait")

		client := newClient()

		// 1. 确定目标 ACR：显式 --acr 优先，否则用服务端亲和性推荐
		var effective *AcrRegistryInfo
		if ns != "" {
			reg, err := resolveAcrByNamespace(client, ns)
			if err != nil {
				return err
			}
			effective = reg
		} else {
			sug, err := suggestAcr(client, image)
			if err != nil {
				return fmt.Errorf("推荐目标 ACR 失败: %w", err)
			}
			regs, err := listRegistries(client)
			if err != nil {
				return err
			}
			for i := range regs {
				if regs[i].ID == sug.SuggestedAcrID {
					effective = &regs[i]
					break
				}
			}
			if effective == nil {
				return fmt.Errorf("服务端推荐的 ACR（id=%d）不存在", sug.SuggestedAcrID)
			}
			fmt.Fprintf(os.Stderr, "目标镜像仓库: %s（%s）\n", aliasOrNamespace(effective.Alias, effective.Namespace), suggestReasonText(sug.SuggestionReason))
		}

		// 2. 提交前查重
		if !force {
			outcome, err := runDedupCheck(client, image, targetTag, effective)
			if err != nil {
				return err
			}
			switch outcome.Action {
			case "block":
				return fmt.Errorf("已拦截: %s", outcome.Message)
			case "info":
				fmt.Fprintf(os.Stderr, "提示: %s\n", outcome.Message)
			}
		}

		// 3. 提交：无 target-tag 走单镜像接口；有则走批量接口单条目（仅后者支持目标 tag）
		var submitted submitResponse
		if targetTag == "" {
			req := submitRequest{
				Images:        []string{image},
				Architecture:  arch,
				Description:   desc,
				AcrRegistryID: effective.ID,
			}
			if err := client.postSyncLimited("/sync/submit", req, &submitted); err != nil {
				return err
			}
		} else {
			req := batchRequest{
				Images: []batchImageItem{{
					SourceImage:  image,
					TargetTag:    targetTag,
					Architecture: arch,
				}},
				AcrRegistryID: effective.ID,
			}
			if err := client.postSyncLimited("/sync/batch", req, &submitted); err != nil {
				return err
			}
		}

		if noWait {
			return emitOrPrint(submitted, func() {
				fmt.Printf("任务已提交: %s\n", submitted.TaskID)
				fmt.Printf("查看进度: dsync task status %s\n", submitted.TaskID)
			})
		}

		// 4. 等待完成
		status, err := waitForTask(client, submitted.TaskID, true)
		if err != nil {
			return err
		}
		return emitOrPrint(status, func() {
			printTaskResult(status)
		})
	},
}

// runDedupCheck 执行提交前查重：check-acr（仓库级）+ 目标 tag 是否已存在。
func runDedupCheck(client *Client, image, targetTag string, effective *AcrRegistryInfo) (dedupOutcome, error) {
	var checkResp checkAcrResponse
	req := map[string]any{
		"images":          []string{image},
		"acr_registry_id": effective.ID,
	}
	if err := client.do("POST", "/sync/check-acr", req, &checkResp); err != nil {
		return dedupOutcome{}, fmt.Errorf("查重请求失败: %w", err)
	}
	if len(checkResp.Data.Items) == 0 {
		return dedupOutcome{}, fmt.Errorf("查重响应为空")
	}
	item := checkResp.Data.Items[0]

	_, srcTag := splitImageTag(image)
	finalTag := targetTag
	if finalTag == "" {
		finalTag = srcTag
	}

	existingTags, tagsKnown := []string(nil), false
	if item.HasAffinity && item.SuggestedAcrID == effective.ID {
		tags, err := fetchTagNames(client, effective.ID, item.RepositoryName)
		if err == nil {
			existingTags, tagsKnown = tags, true
		}
	}
	return decideDedup(item, effective.ID, effective.RegistryURL, effective.Alias, effective.Namespace, finalTag, existingTags, tagsKnown), nil
}

// isTerminalTaskStatus 任务级终态
func isTerminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "partial_success", "paused":
		return true
	}
	return false
}

// getSyncStatus 查询任务状态。
func getSyncStatus(client *Client, taskID string) (*syncStatusResponse, error) {
	var st syncStatusResponse
	if err := client.do("GET", "/sync/status/"+taskID, nil, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// waitForTask 轮询任务直至终态。poll=false 时仅查询一次。
// Ctrl+C 中断时提示任务仍在服务端运行，可用 task status 继续查看。
func waitForTask(client *Client, taskID string, poll bool) (*syncStatusResponse, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		st, err := getSyncStatus(client, taskID)
		if err != nil {
			return nil, err
		}
		if !jsonOut && poll {
			renderProgressLine(st, time.Since(start))
		}
		if !poll || isTerminalTaskStatus(st.Status) {
			if !jsonOut && poll {
				fmt.Fprintln(os.Stderr)
			}
			return st, nil
		}
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\n已中断: 任务仍在服务端运行，可用 dsync task status %s 继续查看\n", taskID)
			os.Exit(130)
		case <-ticker.C:
		}
	}
}

// renderProgressLine 单行刷新展示任务进度（输出到 stderr，避免污染 --json 的 stdout）。
func renderProgressLine(st *syncStatusResponse, elapsed time.Duration) {
	line := fmt.Sprintf("状态=%s 进度=%.0f%% 镜像[待=%d 同步中=%d 成功=%d 失败=%d] 已用时=%s   ",
		colorStatus(st.Status), st.Progress,
		st.Images.Pending, st.Images.Syncing, st.Images.Success, st.Images.Failed,
		humanDuration(elapsed))
	fmt.Fprint(os.Stderr, "\r"+line)
}

// printTaskResult 打印任务终态结果：成功的输出目标镜像地址，失败的输出错误与 Actions 链接。
func printTaskResult(st *syncStatusResponse) {
	fmt.Printf("任务 %s: %s（成功 %d / 失败 %d / 总 %d）\n",
		shortID(st.TaskID), colorStatus(st.Status),
		st.Images.Success, st.Images.Failed, st.TotalImages)

	for _, r := range st.Images.Records {
		switch r.SyncStatus {
		case "success":
			addr := r.ACRImage
			if addr == "" {
				addr = paint(colorYellow, "(目标地址暂未回填)")
			}
			fmt.Printf("  %s %s:%s -> %s\n", paint(colorGreen, "✓"), r.OriginalImage, r.Tag, addr)
		case "failed":
			fmt.Printf("  %s %s:%s 失败: %s\n", paint(colorRed, "✗"), r.OriginalImage, r.Tag, r.ErrorMessage)
		case "skipped":
			fmt.Printf("  %s %s:%s 已跳过\n", paint(colorGray, "-"), r.OriginalImage, r.Tag)
		default:
			fmt.Printf("  %s %s:%s %s\n", paint(colorGray, "-"), r.OriginalImage, r.Tag, r.SyncStatus)
		}
	}
	if st.GitHubActionURL != "" {
		fmt.Printf("Actions: %s\n", st.GitHubActionURL)
	}
	if st.ErrorMessage != "" {
		fmt.Printf("任务错误: %s\n", st.ErrorMessage)
	}
}

func init() {
	syncCmd.Flags().String("acr", "", "目标镜像仓库（别名优先，兼容 namespace；不指定则按亲和性自动选择）")
	syncCmd.Flags().String("target-tag", "", "目标 Tag（默认沿用源镜像 Tag）")
	syncCmd.Flags().String("arch", "", "目标架构，如 amd64 / arm64")
	syncCmd.Flags().String("desc", "", "任务描述")
	syncCmd.Flags().Bool("force", false, "跳过提交前查重拦截")
	syncCmd.Flags().Bool("no-wait", false, "提交后立即返回 task-id，不等待完成")
	rootCmd.AddCommand(syncCmd)
}
