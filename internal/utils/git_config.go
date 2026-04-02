// Package utils 提供通用的工具函数
package utils

import (
	"fmt"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"go.uber.org/zap"
)

// DecryptFunc 解密函数类型
type DecryptFunc func(encryptedValue string) (string, error)

// GetConfigValueWithDecrypt 从数据库获取配置值（支持解密）
//
// 功能说明:
//   - 从system_configs表中查询指定的配置项
//   - 如果配置项被加密，使用提供的解密函数解密后返回
//   - 提供统一的配置获取接口
//
// 参数:
//   - configKey: 配置项的键名
//   - decryptFunc: 解密函数（可为nil，表示不解密）
//
// 返回值:
//   - string: 配置项的值（已解密）
//   - error: 查询或解密过程中的错误
func GetConfigValueWithDecrypt(configKey string, decryptFunc DecryptFunc) (string, error) {
	var systemConfig models.SystemConfig
	err := database.DB.Where("config_key = ?", configKey).First(&systemConfig).Error
	if err != nil {
		return "", fmt.Errorf("配置项 %s 不存在: %w", configKey, err)
	}

	// 如果配置值被加密，需要解密
	if systemConfig.IsEncrypted {
		if decryptFunc == nil {
			return "", fmt.Errorf("配置项 %s 已加密但解密函数未提供", configKey)
		}

		decryptedValue, err := decryptFunc(systemConfig.ConfigValue)
		if err != nil {
			return "", fmt.Errorf("解密配置项 %s 失败: %w", configKey, err)
		}

		logger.Logger.Debug("成功解密配置项", zap.String("config_key", configKey))
		return decryptedValue, nil
	}

	return systemConfig.ConfigValue, nil
}

// GetConfigValueSimple 从数据库获取配置值（简化版本，不处理加密）
//
// 适用于不需要解密的配置项
//
// 参数:
//   - configKey: 配置项的键名
//
// 返回值:
//   - string: 配置项的值
//   - error: 查询过程中的错误
func GetConfigValueSimple(configKey string) (string, error) {
	var systemConfig models.SystemConfig
	err := database.DB.Where("config_key = ?", configKey).First(&systemConfig).Error
	if err != nil {
		return "", fmt.Errorf("配置项 %s 不存在: %w", configKey, err)
	}
	return systemConfig.ConfigValue, nil
}

// GetGitRepositoryType 获取Git仓库类型
//
// 返回值:
//   - string: 仓库类型（github或gitee）
//   - error: 查询过程中的错误
func GetGitRepositoryType() (string, error) {
	var systemConfig models.SystemConfig
	err := database.DB.Where("config_key = ?", "git_repository_type").First(&systemConfig).Error
	if err != nil {
		// 如果配置不存在，默认使用gitee
		logger.Logger.Warn("Git仓库类型配置不存在，使用默认配置", zap.Error(err))
		return "gitee", nil
	}
	return systemConfig.ConfigValue, nil
}
