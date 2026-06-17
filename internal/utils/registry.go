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

func headRegistryRef(ctx context.Context, ref name.Reference, acrRegistryID uint) error {
	_, err := remote.Head(ref, remoteOptionsForACR(ctx, acrRegistryID)...)
	return err
}

// CheckImageExistsInRegistry 检测镜像在注册表中是否存在
//
// 参数:
//   - imageRef: 完整的镜像引用地址（如 registry.cn-hangzhou.aliyuncs.com/namespace/image:tag）
//   - acrRegistryID: ACR 配置 ID，用于选择对应凭据；0 表示使用系统默认配置
//
// 返回值:
//   - bool: 镜像是否存在
//
// 检测机制:
//   - 使用容器注册表API的HEAD请求检测镜像manifest
//   - 支持30秒超时控制
//   - 区分404错误（镜像不存在）和其他错误（网络/权限问题）
func CheckImageExistsInRegistry(imageRef string, acrRegistryID uint) bool {
	exists, err := CheckImageExistsInRegistryWithErr(imageRef, acrRegistryID)
	if err != nil {
		logger.Logger.Warn("检测镜像存在性失败",
			zap.Error(err),
			zap.String("image_ref", imageRef),
			zap.Uint("acr_registry_id", acrRegistryID))
		return false
	}
	if exists {
		logger.Logger.Debug("镜像存在",
			zap.String("image_ref", imageRef),
			zap.Uint("acr_registry_id", acrRegistryID))
	} else {
		logger.Logger.Debug("镜像不存在",
			zap.String("image_ref", imageRef),
			zap.Uint("acr_registry_id", acrRegistryID))
	}
	return exists
}

// CheckImageExistsInRegistryWithErr 检测镜像在注册表中是否存在（带错误返回）
//
// 参数:
//   - imageRef: 完整的镜像引用地址
//   - acrRegistryID: ACR 配置 ID，用于选择对应凭据；0 表示使用系统默认配置
//
// 返回值:
//   - bool: 镜像是否存在
//   - error: 检测过程中的错误（如果有）
func CheckImageExistsInRegistryWithErr(imageRef string, acrRegistryID uint) (bool, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = headRegistryRef(ctx, ref, acrRegistryID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
