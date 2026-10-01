package services

import "context"

// GitServiceInterface 统一的Git服务接口
// 当前唯一消费方为 SyncHandler（updateImagesViaApi 回退路径），
// 仅调用 UpdateImagesFile 触发 images.txt 提交推送
type GitServiceInterface interface {
	UpdateImagesFile(ctx context.Context, newImages []string) (string, error)
}
