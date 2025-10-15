-- ============================================================================
-- 系统配置表结构升级迁移脚本
-- ============================================================================
-- 
-- 本脚本用于为system_configs表添加新字段，支持Git配置数据库化管理
-- 
-- 新增字段：
-- 1. is_encrypted: 标识配置值是否加密存储
-- 2. config_group: 配置分组，用于组织和分类配置项
-- 3. display_order: 显示顺序，用于前端展示排序
-- 
-- 执行前请备份数据库！
-- 
-- ============================================================================

USE docker_sync;

-- 检查表是否存在
SELECT 'Checking system_configs table...' as status;

-- 添加is_encrypted字段
ALTER TABLE system_configs 
ADD COLUMN IF NOT EXISTS is_encrypted BOOLEAN DEFAULT FALSE COMMENT '是否加密存储，用于标识敏感信息';

-- 添加config_group字段
ALTER TABLE system_configs 
ADD COLUMN IF NOT EXISTS config_group VARCHAR(50) DEFAULT 'default' COMMENT '配置分组，用于组织和分类配置项';

-- 添加display_order字段
ALTER TABLE system_configs 
ADD COLUMN IF NOT EXISTS display_order INT DEFAULT 0 COMMENT '显示顺序，用于前端展示时的排序';

-- 为新字段添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_system_configs_group ON system_configs(config_group);
CREATE INDEX IF NOT EXISTS idx_system_configs_order ON system_configs(display_order);

-- 验证字段添加结果
SELECT 'Migration completed. Checking table structure...' as status;
DESCRIBE system_configs;

-- 显示当前表中的数据
SELECT 'Current data in system_configs:' as status;
SELECT id, config_key, config_group, is_encrypted, display_order, created_at 
FROM system_configs 
ORDER BY config_group, display_order, config_key;

SELECT 'Migration script completed successfully!' as status;