-- 添加批量同步支持的数据库迁移
-- 执行时间: 2024-01-XX

-- 1. 更新 sync_tasks 表，添加批量同步相关字段
ALTER TABLE sync_tasks 
ADD COLUMN description VARCHAR(500) DEFAULT '' COMMENT '任务描述',
ADD COLUMN max_concurrent INT DEFAULT 3 COMMENT '最大并发数',
ADD COLUMN total_images INT DEFAULT 0 COMMENT '总镜像数',
ADD COLUMN completed_images INT DEFAULT 0 COMMENT '已完成镜像数',
ADD COLUMN failed_images INT DEFAULT 0 COMMENT '失败镜像数',
ADD COLUMN auto_retry BOOLEAN DEFAULT FALSE COMMENT '是否自动重试',
ADD COLUMN retry_count INT DEFAULT 0 COMMENT '重试次数',
ADD COLUMN current_retry INT DEFAULT 0 COMMENT '当前重试次数',
ADD COLUMN progress DECIMAL(5,2) DEFAULT 0.00 COMMENT '进度百分比';

-- 2. 更新 sync_tasks 表的 status 枚举，添加 paused 状态
ALTER TABLE sync_tasks 
MODIFY COLUMN status ENUM('pending','running','completed','failed','paused') DEFAULT 'pending';

-- 3. 更新 image_sync_records 表，添加新字段
ALTER TABLE image_sync_records 
ADD COLUMN priority INT DEFAULT 0 COMMENT '优先级',
ADD COLUMN retry_count INT DEFAULT 0 COMMENT '重试次数',
ADD COLUMN max_retries INT DEFAULT 3 COMMENT '最大重试次数',
ADD COLUMN started_at TIMESTAMP NULL COMMENT '开始时间',
ADD COLUMN completed_at TIMESTAMP NULL COMMENT '完成时间',
ADD COLUMN duration BIGINT DEFAULT 0 COMMENT '同步耗时（秒）',
ADD COLUMN image_size BIGINT DEFAULT 0 COMMENT '镜像大小（字节）';

-- 4. 更新 image_sync_records 表的 sync_status 枚举，添加新状态
ALTER TABLE image_sync_records 
MODIFY COLUMN sync_status ENUM('pending','syncing','success','failed','retrying','skipped') DEFAULT 'pending';

-- 5. 添加新的索引以提高查询性能
CREATE INDEX idx_sync_tasks_status_progress ON sync_tasks(status, progress);
CREATE INDEX idx_sync_tasks_total_completed ON sync_tasks(total_images, completed_images);
CREATE INDEX idx_image_sync_records_priority ON image_sync_records(priority DESC);
CREATE INDEX idx_image_sync_records_retry ON image_sync_records(retry_count, max_retries);
CREATE INDEX idx_image_sync_records_duration ON image_sync_records(duration);

-- 6. 更新现有记录的默认值
UPDATE sync_tasks SET 
    max_concurrent = 3,
    total_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id
    ),
    completed_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id 
        AND sync_status = 'success'
    ),
    failed_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id 
        AND sync_status = 'failed'
    )
WHERE max_concurrent = 0;

-- 7. 计算现有任务的进度
UPDATE sync_tasks SET 
    progress = CASE 
        WHEN total_images > 0 THEN 
            ROUND((completed_images * 100.0 / total_images), 2)
        ELSE 0.00 
    END
WHERE progress = 0.00 AND total_images > 0;

-- 8. 为现有的镜像记录设置默认的最大重试次数
UPDATE image_sync_records SET max_retries = 3 WHERE max_retries = 0;

COMMIT;