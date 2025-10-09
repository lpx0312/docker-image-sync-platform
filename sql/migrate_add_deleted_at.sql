-- 数据库迁移脚本：添加deleted_at字段
-- 用于为现有的数据库表添加软删除支持

USE docker_sync;

-- 为image_sync_records表添加deleted_at字段
ALTER TABLE image_sync_records 
ADD COLUMN deleted_at TIMESTAMP NULL COMMENT '删除时间',
ADD INDEX idx_deleted_at (deleted_at);

-- 为sync_tasks表添加deleted_at字段
ALTER TABLE sync_tasks 
ADD COLUMN deleted_at TIMESTAMP NULL COMMENT '删除时间',
ADD INDEX idx_deleted_at (deleted_at);

-- 为system_configs表添加deleted_at字段
ALTER TABLE system_configs 
ADD COLUMN deleted_at TIMESTAMP NULL COMMENT '删除时间',
ADD INDEX idx_deleted_at (deleted_at);

-- 显示迁移完成信息
SELECT 'Migration completed: deleted_at fields added to all tables' AS status;