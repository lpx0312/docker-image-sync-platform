// Package utils 提供通用的工具函数
//
// 本包包含镜像同步平台的核心工具函数，避免代码重复
package utils

import (
	"fmt"
	"strings"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/models"
)

// GenerateACRImage 生成阿里云ACR镜像地址（简化版本，不含架构信息）
func GenerateACRImage(originalImage, tag string) string {
	return GenerateACRImageWithArchitecture(originalImage, tag, "")
}

// GenerateACRImageWithArchitecture 生成带架构信息的阿里云ACR镜像地址
//
// 这是核心的ACR地址生成器，支持多架构镜像的标签生成
//
// 参数:
//   - originalImage: 原始镜像名称（如 nginx、library/nginx）
//   - tag: 镜像标签（如 latest、1.20）
//   - architecture: 目标架构（如 amd64、arm64、linux/arm64）
//
// 返回值:
//   - string: 生成的ACR镜像完整地址，格式为 registry/namespace/image:tag[-arch-suffix]
//
// 地址生成规则:
//   - 默认架构(amd64)不添加后缀
//   - 其他架构添加 -linux-架构名 后缀
//   - 支持简化架构名(arm64)和完整平台名(linux/arm64)
func GenerateACRImageWithArchitecture(originalImage, tag, architecture string) string {
	// 从数据库获取阿里云注册表配置
	var registryConfig models.SystemConfig
	database.DB.Where("config_key = ?", "aliyun_registry").First(&registryConfig)

	registry := registryConfig.ConfigValue
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}

	// 从数据库获取阿里云命名空间配置
	var namespaceConfig models.SystemConfig
	database.DB.Where("config_key = ?", "aliyun_namespace").First(&namespaceConfig)

	namespace := namespaceConfig.ConfigValue
	if namespace == "" {
		namespace = "lpx03"
	}

	// 提取镜像的基础名称
	// 处理带命名空间的镜像名（如 library/nginx -> nginx）
	imageName := originalImage
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		imageName = parts[len(parts)-1]
	}

	// 为非默认架构生成标签后缀
	architectureSuffix := ""
	if architecture != "" && architecture != "amd64" {
		var platform string
		switch architecture {
		case "arm64":
			platform = "linux/arm64"
		case "arm":
			platform = "linux/arm"
		case "386":
			platform = "linux/386"
		default:
			if strings.Contains(architecture, "/") {
				platform = architecture
			} else {
				platform = "linux/" + architecture
			}
		}
		// 将 linux/arm64 转换为 -linux-arm64
		architectureSuffix = "-" + strings.ReplaceAll(platform, "/", "-")
	}

	// 确保标签不为空
	finalTag := tag
	if finalTag == "" {
		finalTag = "latest"
	}

	// 最终标签：tag + architectureSuffix
	finalTagWithArch := finalTag + architectureSuffix

	// 按照阿里云ACR的地址格式组装完整地址
	return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, imageName, finalTagWithArch)
}
