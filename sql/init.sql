/**
 * Docker镜像同步平台数据库初始化脚本
 * 
 * 功能说明：
 * - 创建项目所需的数据库和表结构
 * - 设置合适的字符集和排序规则
 * - 创建必要的索引以优化查询性能
 * - 插入系统默认配置数据
 * 
 * 数据库设计：
 * - image_sync_records: 镜像同步记录表，存储每个镜像的同步状态
 * - sync_tasks: 同步任务表，管理批量同步任务
 * - system_configs: 系统配置表，存储可配置的系统参数
 * 
 * 字符集配置：
 * - 使用 utf8mb4 字符集，支持完整的UTF-8字符（包括emoji）
 * - 使用 utf8mb4_unicode_ci 排序规则，提供更好的国际化支持
 * 
 * 执行说明：
 * - 此脚本在Docker容器启动时自动执行
 * - 支持重复执行，使用 IF NOT EXISTS 避免重复创建
 * - 建议在生产环境部署前先在测试环境验证
 * 
 * @author Docker Image Sync Platform
 * @version 1.0.0
 * @created 2024-01-01
 */

-- ========================================================================
-- 数据库创建
-- ========================================================================

-- 创建主数据库
-- 使用utf8mb4字符集确保支持完整的Unicode字符集
CREATE DATABASE IF NOT EXISTS docker_sync 
    CHARACTER SET utf8mb4 
    COLLATE utf8mb4_unicode_ci;

-- 切换到目标数据库
USE docker_sync;

-- ========================================================================
-- 镜像同步记录表 (image_sync_records)
-- ========================================================================
-- 功能：存储每个Docker镜像的同步状态和详细信息
-- 用途：跟踪镜像从源仓库到阿里云ACR的同步过程
-- 特性：支持软删除、状态跟踪、错误记录
CREATE TABLE IF NOT EXISTS image_sync_records (
    -- 主键ID，自增长
    id INT AUTO_INCREMENT PRIMARY KEY,
    
    -- 原始镜像地址（必填）
    -- 格式：registry/namespace/image:tag 或 image:tag
    -- 示例：nginx:latest, docker.io/library/redis:alpine
    original_image VARCHAR(500) NOT NULL COMMENT '原始镜像地址',
    
    -- 阿里云ACR目标地址（同步后生成）
    -- 格式：registry.cn-hangzhou.aliyuncs.com/namespace/image:tag
    acr_image VARCHAR(500) COMMENT '阿里云ACR地址',
    
    -- 镜像标签
    -- 从original_image中解析出来，如：latest, v1.0, alpine
    tag VARCHAR(100) COMMENT '标签',
    
    -- 目标架构
    -- 支持：amd64, arm64, arm/v7等，默认amd64
    architecture VARCHAR(50) COMMENT '架构',
    
    -- 同步状态枚举
    -- pending: 等待同步, syncing: 同步中, success: 成功, failed: 失败
    sync_status ENUM('pending', 'syncing', 'success', 'failed') DEFAULT 'pending' COMMENT '同步状态',
    
    -- 错误信息（失败时记录）
    -- 存储详细的错误描述，便于问题排查
    error_message TEXT COMMENT '错误信息',
    
    -- 关联的批量任务ID
    -- 用于将多个镜像同步记录关联到同一个批量任务
    task_id VARCHAR(100) COMMENT '关联的任务ID',
    
    -- 时间戳字段
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间（软删除）',
    
    -- 索引定义（优化查询性能）
    INDEX idx_original_image (original_image),    -- 按原始镜像查询
    INDEX idx_sync_status (sync_status),          -- 按状态筛选
    INDEX idx_task_id (task_id),                  -- 按任务ID查询
    INDEX idx_created_at (created_at),            -- 按时间排序
    INDEX idx_deleted_at (deleted_at)             -- 软删除查询优化
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='镜像同步记录表';

-- ========================================================================
-- 同步任务表 (sync_tasks)
-- ========================================================================
-- 功能：管理批量镜像同步任务的生命周期
-- 用途：跟踪GitHub Actions工作流的执行状态和结果
-- 特性：支持批量操作、GitHub集成、任务状态管理
CREATE TABLE IF NOT EXISTS sync_tasks (
    -- 主键ID，自增长
    id INT AUTO_INCREMENT PRIMARY KEY,
    
    -- 唯一任务标识符（必填，唯一）
    -- 格式：通常使用UUID或时间戳生成
    -- 示例：task_20240101_123456, uuid-v4格式
    task_id VARCHAR(100) UNIQUE NOT NULL COMMENT '任务ID',
    
    -- 本次同步的镜像列表（JSON格式）
    -- 存储镜像名称、标签、架构等信息的JSON数组
    -- 示例：[{"image":"nginx:latest","arch":"amd64"},...]
    images_json TEXT COMMENT '本次同步的镜像列表JSON',
    
    -- 任务执行状态枚举
    -- pending: 待执行, running: 执行中, completed: 已完成, failed: 失败
    status ENUM('pending', 'running', 'completed', 'failed') DEFAULT 'pending' COMMENT '任务状态',
    
    -- GitHub Actions相关字段
    -- GitHub Action工作流的完整URL链接
    github_action_url VARCHAR(500) COMMENT 'GitHub Action链接',
    
    -- GitHub Actions运行实例ID
    -- 用于通过API查询具体的运行状态和日志
    github_run_id VARCHAR(100) COMMENT 'GitHub Action运行ID',
    
    -- Git提交的SHA值
    -- 记录触发此次同步的具体代码版本
    commit_sha VARCHAR(100) COMMENT 'Git提交SHA',
    
    -- 任务执行时间记录
    started_at TIMESTAMP NULL COMMENT '开始时间',
    completed_at TIMESTAMP NULL COMMENT '完成时间',
    
    -- 错误信息（失败时记录）
    -- 存储任务失败的详细原因和错误堆栈
    error_message TEXT COMMENT '错误信息',
    
    -- 标准时间戳字段
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间（软删除）',
    
    -- 索引定义（优化查询性能）
    INDEX idx_task_id (task_id),                  -- 按任务ID查询（唯一索引）
    INDEX idx_status (status),                    -- 按状态筛选任务
    INDEX idx_created_at (created_at),            -- 按创建时间排序
    INDEX idx_deleted_at (deleted_at)             -- 软删除查询优化
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='同步任务表';

-- ========================================================================
-- 系统配置表 (system_configs)
-- ========================================================================
-- 功能：存储系统运行时的可配置参数
-- 用途：动态配置系统行为，无需重启服务即可生效
-- 特性：支持热更新、版本控制、配置验证
CREATE TABLE IF NOT EXISTS system_configs (
    -- 主键ID，自增长
    id INT AUTO_INCREMENT PRIMARY KEY,
    
    -- 配置键名（必填，唯一）
    -- 格式：使用下划线分隔的小写字母，如：aliyun_registry_prefix
    -- 命名规范：模块_功能_属性
    config_key VARCHAR(100) UNIQUE NOT NULL COMMENT '配置键',
    
    -- 配置值（支持各种数据类型）
    -- 可存储字符串、数字、JSON等格式的配置值
    -- 应用层负责类型转换和验证
    config_value TEXT COMMENT '配置值',
    
    -- 配置项的详细描述
    -- 说明配置的作用、取值范围、注意事项等
    description VARCHAR(500) COMMENT '配置描述',
    
    -- 标准时间戳字段
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间（软删除）',
    
    -- 索引定义（优化查询性能）
    INDEX idx_config_key (config_key),            -- 按配置键查询（唯一索引）
    INDEX idx_deleted_at (deleted_at)             -- 软删除查询优化
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统配置表';

-- ========================================================================
-- 默认系统配置数据
-- ========================================================================
-- 插入系统运行所需的默认配置项
-- 使用 ON DUPLICATE KEY UPDATE 确保可重复执行
INSERT INTO system_configs (config_key, config_value, description) VALUES
-- 阿里云镜像仓库配置
('aliyun_registry_prefix', 'registry.cn-hangzhou.aliyuncs.com', '阿里云镜像仓库前缀地址'),

-- 同步任务配置
('sync_check_interval', '30', '同步状态检查间隔时间（秒），建议范围：10-300'),

-- 并发控制配置
('max_concurrent_syncs', '5', '最大并发同步任务数量，建议根据服务器性能调整')

-- 如果配置已存在则更新值（支持配置升级）
ON DUPLICATE KEY UPDATE config_value = VALUES(config_value);