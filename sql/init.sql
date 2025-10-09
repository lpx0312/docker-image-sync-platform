-- Docker镜像同步平台数据库初始化脚本

-- 创建数据库
CREATE DATABASE IF NOT EXISTS docker_sync CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE docker_sync;

-- 镜像同步记录表
CREATE TABLE IF NOT EXISTS image_sync_records (
    id INT AUTO_INCREMENT PRIMARY KEY,
    original_image VARCHAR(500) NOT NULL COMMENT '原始镜像地址',
    acr_image VARCHAR(500) COMMENT '阿里云ACR地址',
    tag VARCHAR(100) COMMENT '标签',
    architecture VARCHAR(50) COMMENT '架构',
    sync_status ENUM('pending', 'syncing', 'success', 'failed') DEFAULT 'pending' COMMENT '同步状态',
    error_message TEXT COMMENT '错误信息',
    task_id VARCHAR(100) COMMENT '关联的任务ID',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间',
    INDEX idx_original_image (original_image),
    INDEX idx_sync_status (sync_status),
    INDEX idx_task_id (task_id),
    INDEX idx_created_at (created_at),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='镜像同步记录表';

-- 同步任务表
CREATE TABLE IF NOT EXISTS sync_tasks (
    id INT AUTO_INCREMENT PRIMARY KEY,
    task_id VARCHAR(100) UNIQUE NOT NULL COMMENT '任务ID',
    images_json TEXT COMMENT '本次同步的镜像列表JSON',
    status ENUM('pending', 'running', 'completed', 'failed') DEFAULT 'pending' COMMENT '任务状态',
    github_action_url VARCHAR(500) COMMENT 'GitHub Action链接',
    github_run_id VARCHAR(100) COMMENT 'GitHub Action运行ID',
    commit_sha VARCHAR(100) COMMENT 'Git提交SHA',
    started_at TIMESTAMP NULL COMMENT '开始时间',
    completed_at TIMESTAMP NULL COMMENT '完成时间',
    error_message TEXT COMMENT '错误信息',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间',
    INDEX idx_task_id (task_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='同步任务表';

-- 系统配置表
CREATE TABLE IF NOT EXISTS system_configs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    config_key VARCHAR(100) UNIQUE NOT NULL COMMENT '配置键',
    config_value TEXT COMMENT '配置值',
    description VARCHAR(500) COMMENT '配置描述',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间',
    INDEX idx_config_key (config_key),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统配置表';

-- 插入默认配置
INSERT INTO system_configs (config_key, config_value, description) VALUES
('aliyun_registry_prefix', 'registry.cn-hangzhou.aliyuncs.com', '阿里云镜像仓库前缀'),
('sync_check_interval', '30', '同步状态检查间隔（秒）'),
('max_concurrent_syncs', '5', '最大并发同步数量')
ON DUPLICATE KEY UPDATE config_value = VALUES(config_value);