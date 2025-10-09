-- 为现有数据添加默认架构值
-- 执行时间：2024年

-- 更新现有记录的架构字段为默认值 amd64
UPDATE image_sync_records 
SET architecture = 'amd64' 
WHERE architecture IS NULL OR architecture = '';

-- 修改表结构，设置默认值
ALTER TABLE image_sync_records 
MODIFY COLUMN architecture VARCHAR(50) DEFAULT 'amd64';

-- 为同步任务表也添加架构支持（如果需要的话）
-- ALTER TABLE sync_tasks ADD COLUMN default_architecture VARCHAR(50) DEFAULT 'amd64';