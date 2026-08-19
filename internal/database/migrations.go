package database

import (
	"fmt"
	"strings"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/logger"

	"go.uber.org/zap"
)

// RunMigrations 执行数据库迁移
func RunMigrations() {
	logger.Logger.Info("开始执行数据库迁移...")

	// 自动迁移 ACR 配置表
	if err := DB.AutoMigrate(&models.AcrRegistry{}); err != nil {
		logger.Logger.Error("ACR配置表迁移失败", zap.Error(err))
	} else {
		logger.Logger.Info("ACR配置表迁移成功")
	}

	// 自动迁移 ACR 镜像仓库表
	if err := DB.AutoMigrate(&models.AcrRepository{}); err != nil {
		logger.Logger.Error("ACR镜像仓库表迁移失败", zap.Error(err))
	} else {
		logger.Logger.Info("ACR镜像仓库表迁移成功")
	}

	// 老数据回填别名（幂等：仅处理 alias 为空的记录）
	backfillAcrRegistryAlias()

	logger.Logger.Info("数据库迁移完成")
}

// backfillAcrRegistryAlias 为升级前没有别名的记录生成别名。
// 规则：默认取 namespace；与其他别名冲突时追加类型后缀（如 lpx03-SWR），
// 仍冲突则追加记录 ID。按 ID 顺序处理保证幂等且结果稳定。
func backfillAcrRegistryAlias() {
	var registries []models.AcrRegistry
	if err := DB.Order("id ASC").Find(&registries).Error; err != nil {
		logger.Logger.Error("回填别名失败：查询记录失败", zap.Error(err))
		return
	}

	used := make(map[string]bool, len(registries))
	changed := 0
	for i := range registries {
		r := &registries[i]
		if r.Alias != "" {
			used[r.Alias] = true
			continue
		}

		base := strings.TrimSpace(r.Namespace)
		if base == "" {
			base = fmt.Sprintf("registry-%d", r.ID)
		}
		alias := base
		if used[alias] {
			suffix := r.RegistryType
			if suffix == "" {
				suffix = models.RegistryTypeACR
			}
			alias = base + "-" + strings.ToUpper(suffix)
			if used[alias] {
				alias = fmt.Sprintf("%s-%d", base, r.ID)
			}
		}
		used[alias] = true

		if err := DB.Model(r).Update("alias", alias).Error; err != nil {
			logger.Logger.Error("回填别名失败",
				zap.Uint("id", r.ID), zap.String("alias", alias), zap.Error(err))
			continue
		}
		changed++
	}

	if changed > 0 {
		logger.Logger.Info("已为老数据回填镜像仓库别名", zap.Int("count", changed))
	}
}
