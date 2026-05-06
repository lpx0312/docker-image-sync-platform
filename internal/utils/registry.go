// Package utils 提供通用的工具函数
package utils

import (
	"context"
	"strings"
	"time"

	"docker-image-sync-platform/internal/logger"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.uber.org/zap"
)

func headRegistryRef(ctx context.Context, ref name.Reference) error {
	_, err := remote.Head(ref, remoteOptions(ctx)...)
	return err
}

// CheckImageExistsInRegistry 检测镜像在注册表中是否存在
//
// 参数:
//   - imageRef: 完整的镜像引用地址（如 registry.cn-hangzhou.aliyuncs.com/namespace/image:tag）
//
// 返回值:
//   - bool: 镜像是否存在
//
// 检测机制:
//   - 使用容器注册表API的HEAD请求检测镜像manifest
//   - 支持30秒超时控制
//   - 区分404错误（镜像不存在）和其他错误（网络/权限问题）
func CheckImageExistsInRegistry(imageRef string) bool {
	// 解析镜像引用
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		logger.Logger.Error("解析镜像引用失败",
			zap.Error(err),
			zap.String("image_ref", imageRef))
		return false
	}

	// 创建带30秒超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 使用HEAD请求获取镜像manifest（含 ACR 凭据）
	err = headRegistryRef(ctx, ref)
	if err != nil {
		// 404错误表示镜像不存在
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			logger.Logger.Debug("镜像不存在",
				zap.String("image_ref", imageRef))
			return false
		}

		// 其他错误也认为镜像不存在
		logger.Logger.Warn("检测镜像存在性失败",
			zap.Error(err),
			zap.String("image_ref", imageRef))
		return false
	}

	logger.Logger.Debug("镜像存在",
		zap.String("image_ref", imageRef))
	return true
}

// CheckImageExistsInRegistryWithErr 检测镜像在注册表中是否存在（带错误返回）
//
// 参数:
//   - imageRef: 完整的镜像引用地址
//
// 返回值:
//   - bool: 镜像是否存在
//   - error: 检测过程中的错误（如果有）
func CheckImageExistsInRegistryWithErr(imageRef string) (bool, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = headRegistryRef(ctx, ref)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
