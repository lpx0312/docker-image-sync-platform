-- ========================================================================
-- 数据库迁移脚本：添加批量同步支持
-- ========================================================================
-- 版本：v2.0.0
-- 创建时间：2024年
-- 目的：扩展数据库表结构以支持批量镜像同步功能
-- 
-- 新增功能：
-- 1. 批量任务管理：支持一次性同步多个镜像
-- 2. 并发控制：可配置最大并发同步数量
-- 3. 进度跟踪：实时显示同步进度和统计信息
-- 4. 重试机制：支持失败任务的自动重试
-- 5. 优先级调度：支持镜像同步的优先级排序
-- 6. 性能监控：记录同步耗时和镜像大小信息
-- 
-- 执行前提：
-- - 确保数据库连接正常
-- - 建议在维护窗口期间执行
-- - 执行前请备份数据库
-- - 此迁移会修改现有数据，请谨慎操作
-- ========================================================================

-- ========================================================================
-- 1. 扩展同步任务表 (sync_tasks) - 添加批量同步字段
-- ========================================================================
-- 功能：为同步任务表添加批量处理和进度跟踪能力
ALTER TABLE sync_tasks 
-- 任务描述信息
ADD COLUMN description VARCHAR(500) DEFAULT '' COMMENT '任务描述（用户自定义的任务说明）',

-- 并发控制配置
ADD COLUMN max_concurrent INT DEFAULT 3 COMMENT '最大并发同步数（建议范围：1-10）',

-- 统计信息字段
ADD COLUMN total_images INT DEFAULT 0 COMMENT '总镜像数量（任务包含的镜像总数）',
ADD COLUMN completed_images INT DEFAULT 0 COMMENT '已完成镜像数（成功同步的镜像数量）',
ADD COLUMN failed_images INT DEFAULT 0 COMMENT '失败镜像数（同步失败的镜像数量）',

-- 重试机制配置
ADD COLUMN auto_retry BOOLEAN DEFAULT FALSE COMMENT '是否启用自动重试（失败时自动重新尝试）',
ADD COLUMN retry_count INT DEFAULT 0 COMMENT '总重试次数（任务级别的重试计数）',
ADD COLUMN current_retry INT DEFAULT 0 COMMENT '当前重试轮次（正在进行的重试次数）',

-- 进度跟踪
ADD COLUMN progress DECIMAL(5,2) DEFAULT 0.00 COMMENT '任务进度百分比（0.00-100.00）';

-- ========================================================================
-- 2. 扩展任务状态枚举 - 添加暂停状态
-- ========================================================================
-- 功能：支持任务的暂停和恢复操作
ALTER TABLE sync_tasks 
MODIFY COLUMN status ENUM('pending','running','completed','failed','paused') DEFAULT 'pending' 
COMMENT '任务状态（pending:等待中, running:运行中, completed:已完成, failed:失败, paused:已暂停）';

-- ========================================================================
-- 3. 扩展镜像同步记录表 (image_sync_records) - 添加增强字段
-- ========================================================================
-- 功能：为单个镜像同步记录添加更详细的跟踪信息
ALTER TABLE image_sync_records 
-- 优先级调度
ADD COLUMN priority INT DEFAULT 0 COMMENT '同步优先级（数值越大优先级越高，范围：0-100）',

-- 重试机制
ADD COLUMN retry_count INT DEFAULT 0 COMMENT '已重试次数（当前镜像的重试计数）',
ADD COLUMN max_retries INT DEFAULT 3 COMMENT '最大重试次数（失败后的最大重试限制）',

-- 时间跟踪
ADD COLUMN started_at TIMESTAMP NULL COMMENT '同步开始时间（实际开始同步的时间戳）',
ADD COLUMN completed_at TIMESTAMP NULL COMMENT '同步完成时间（成功或失败的完成时间）',

-- 性能监控
ADD COLUMN duration BIGINT DEFAULT 0 COMMENT '同步耗时（单位：秒，用于性能分析）',
ADD COLUMN image_size BIGINT DEFAULT 0 COMMENT '镜像大小（单位：字节，用于统计和分析）';

-- ========================================================================
-- 4. 扩展同步状态枚举 - 添加新的同步状态
-- ========================================================================
-- 功能：支持更细粒度的同步状态跟踪
ALTER TABLE image_sync_records 
MODIFY COLUMN sync_status ENUM('pending','syncing','success','failed','retrying','skipped') DEFAULT 'pending'
COMMENT '同步状态（pending:等待中, syncing:同步中, success:成功, failed:失败, retrying:重试中, skipped:已跳过）';

-- ========================================================================
-- 5. 性能优化索引 - 提高查询效率
-- ========================================================================
-- 功能：为新增字段创建索引，优化常用查询的性能
-- 任务状态和进度查询优化（用于任务列表和进度监控）
CREATE INDEX idx_sync_tasks_status_progress ON sync_tasks(status, progress);

-- 任务统计信息查询优化（用于批量任务的统计展示）
CREATE INDEX idx_sync_tasks_total_completed ON sync_tasks(total_images, completed_images);

-- 优先级排序查询优化（用于按优先级调度同步任务）
CREATE INDEX idx_image_sync_records_priority ON image_sync_records(priority DESC);

-- 重试机制查询优化（用于重试逻辑和失败分析）
CREATE INDEX idx_image_sync_records_retry ON image_sync_records(retry_count, max_retries);

-- 性能分析查询优化（用于同步耗时统计和性能监控）
CREATE INDEX idx_image_sync_records_duration ON image_sync_records(duration);

-- ========================================================================
-- 6. 数据迁移 - 更新现有记录的默认值
-- ========================================================================
-- 功能：为现有数据设置合理的默认值，确保向后兼容性
-- 为现有任务设置批量同步的默认配置
UPDATE sync_tasks SET 
    -- 设置默认并发数
    max_concurrent = 3,
    
    -- 计算任务包含的总镜像数
    total_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id
    ),
    
    -- 计算已成功完成的镜像数
    completed_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id 
        AND sync_status = 'success'
    ),
    
    -- 计算失败的镜像数
    failed_images = (
        SELECT COUNT(*) 
        FROM image_sync_records 
        WHERE image_sync_records.task_id = sync_tasks.task_id 
        AND sync_status = 'failed'
    )
-- 只更新尚未设置并发数的任务（避免重复更新）
WHERE max_concurrent = 0;

-- ========================================================================
-- 7. 进度计算 - 为现有任务计算完成进度
-- ========================================================================
-- 功能：根据已完成和总镜像数计算任务进度百分比
UPDATE sync_tasks SET 
    progress = CASE 
        WHEN total_images > 0 THEN 
            ROUND((completed_images * 100.0 / total_images), 2)
        ELSE 0.00 
    END
WHERE progress = 0.00 AND total_images > 0;

-- ========================================================================
-- 8. 重试配置 - 为现有镜像记录设置默认重试次数
-- ========================================================================
-- 功能：确保所有镜像记录都有合理的重试配置
UPDATE image_sync_records SET max_retries = 3 WHERE max_retries = 0;

-- ========================================================================
-- 迁移完成确认
-- ========================================================================
-- 提交所有更改并确认迁移成功完成
COMMIT;