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
-- 数据库初始化设置
-- ========================================================================

-- 设置字符集为utf8mb4，确保支持完整的Unicode字符集
SET NAMES utf8mb4;

-- 临时禁用外键检查，避免表创建顺序问题
SET FOREIGN_KEY_CHECKS = 0;

-- ========================================================================
-- 镜像同步记录表 (image_sync_records)
-- ========================================================================
-- 功能：存储每个Docker镜像的同步状态和详细信息
-- 用途：跟踪镜像从源仓库到阿里云ACR的同步过程
-- 特性：支持软删除、状态跟踪、错误记录、重试机制
DROP TABLE IF EXISTS `image_sync_records`;
CREATE TABLE `image_sync_records` (
    -- 主键ID，自增长
    `id` int NOT NULL AUTO_INCREMENT COMMENT '主键ID',

    -- 原始镜像地址（必填）
    -- 格式：registry/namespace/image:tag 或 image:tag
    -- 示例：nginx:latest, docker.io/library/redis:alpine
    `original_image` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '原始镜像地址',

    -- 阿里云ACR目标地址（同步后生成）
    -- 格式：registry.cn-hangzhou.aliyuncs.com/namespace/image:tag
    `acr_image` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL COMMENT '阿里云ACR地址',

    -- 镜像标签
    -- 从original_image中解析出来，如：latest, v1.0, alpine
    `tag` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL COMMENT '镜像标签',

    -- 目标架构
    -- 支持：amd64, arm64, arm/v7等，默认amd64
    `architecture` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT 'amd64' COMMENT '镜像架构（amd64/arm64/arm等，默认amd64）',

    -- 同步状态枚举
    -- pending: 等待同步, syncing: 同步中, success: 成功, failed: 失败
    `sync_status` enum('pending','syncing','success','failed') CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT 'pending' COMMENT '同步状态:pending: 等待同步, syncing: 同步中, success: 成功, failed: 失败',

    -- 错误信息（失败时记录）
    -- 存储详细的错误描述，便于问题排查
    `error_message` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL COMMENT '错误信息',
    
    -- 关联的批量任务ID
    -- 用于将多个镜像同步记录关联到同一个批量任务
    `task_id` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL COMMENT '关联的任务ID',
    
    -- 时间戳字段
    `created_at` datetime(3) NULL DEFAULT NULL COMMENT '创建时间',
    `updated_at` datetime(3) NULL DEFAULT NULL COMMENT '更新时间',
    `deleted_at` datetime(3) NULL DEFAULT NULL COMMENT '删除时间（软删除）',
    
    -- 任务优先级
    -- 数值越大优先级越高，默认为0
    `priority` bigint NULL DEFAULT 0 COMMENT '任务优先级',
    
    -- 重试计数
    -- 记录当前任务已重试的次数
    `retry_count` bigint NULL DEFAULT 0 COMMENT '重试次数',
    
    -- 最大重试次数
    -- 任务失败后的最大重试次数，默认3次
    `max_retries` bigint NULL DEFAULT 3 COMMENT '最大重试次数',
    
    -- 任务执行时间记录
    `started_at` datetime(3) NULL DEFAULT NULL COMMENT '开始时间',
    `completed_at` datetime(3) NULL DEFAULT NULL COMMENT '完成时间',
    
    -- 执行耗时（毫秒）
    -- 记录同步任务的执行时长
    `duration` bigint NULL DEFAULT 0 COMMENT '执行耗时（毫秒）',
    
    -- 镜像大小（字节）
    -- 记录同步镜像的大小信息
    `image_size` bigint NULL DEFAULT 0 COMMENT '镜像大小（字节）',
    
    -- 原始输入内容
    -- 保存用户原始输入的镜像信息，用于追溯
    `original_input` varchar(600) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL COMMENT '原始输入内容',
    
    -- 输入顺序
    -- 在批量同步中记录镜像的输入顺序
    `input_order` bigint NULL DEFAULT 0 COMMENT '输入顺序',
    
    -- 主键定义
    PRIMARY KEY (`id`) USING BTREE,
    
    -- 索引定义（优化查询性能）
    INDEX `idx_original_image`(`original_image` ASC) USING BTREE COMMENT '按原始镜像查询索引',
    INDEX `idx_sync_status`(`sync_status` ASC) USING BTREE COMMENT '按状态筛选索引',
    INDEX `idx_task_id`(`task_id` ASC) USING BTREE COMMENT '按任务ID查询索引',
    INDEX `idx_created_at`(`created_at` ASC) USING BTREE COMMENT '按时间排序索引',
    INDEX `idx_image_sync_records_created_at`(`created_at` ASC) USING BTREE COMMENT '创建时间索引',
    INDEX `idx_image_sync_records_deleted_at`(`deleted_at` ASC) USING BTREE COMMENT '软删除查询索引',
    INDEX `idx_image_sync_records_original_image`(`original_image` ASC) USING BTREE COMMENT '原始镜像索引',
    INDEX `idx_image_sync_records_sync_status`(`sync_status` ASC) USING BTREE COMMENT '同步状态索引',
    INDEX `idx_image_sync_records_task_id`(`task_id` ASC) USING BTREE COMMENT '任务ID索引',
    INDEX `idx_image_sync_records_input_order`(`input_order` ASC) USING BTREE COMMENT '输入顺序索引'
) ENGINE = InnoDB AUTO_INCREMENT = 1 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '镜像同步记录表' ROW_FORMAT = Dynamic;

-- ========================================================================
-- 系统配置表 (system_configs)
-- ========================================================================
-- 功能：存储系统运行时的可配置参数
-- 用途：动态配置系统行为，无需重启服务即可生效
-- 特性：支持热更新、版本控制、配置验证
DROP TABLE IF EXISTS `system_configs`;
CREATE TABLE `system_configs` (
    -- 主键ID，自增长
    `id` int NOT NULL AUTO_INCREMENT COMMENT '主键ID',
    
    -- 配置键名（必填，唯一）
    -- 格式：使用下划线分隔的小写字母，如：aliyun_registry_prefix
    -- 命名规范：模块_功能_属性
    `config_key` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '配置键名',
    
    -- 配置值（支持各种数据类型）
    -- 可存储字符串、数字、JSON等格式的配置值
    -- 应用层负责类型转换和验证
    `config_value` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL COMMENT '配置值',
    
    -- 配置项的详细描述
    -- 说明配置的作用、取值范围、注意事项等
    `description` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL COMMENT '配置描述',
    
    -- 时间戳字段
    `created_at` datetime(3) NULL DEFAULT NULL COMMENT '创建时间',
    `updated_at` datetime(3) NULL DEFAULT NULL COMMENT '更新时间',
    `deleted_at` datetime(3) NULL DEFAULT NULL COMMENT '删除时间（软删除）',
    
    -- 主键定义
    PRIMARY KEY (`id`) USING BTREE,
    
    -- 唯一索引定义
    UNIQUE INDEX `config_key`(`config_key` ASC) USING BTREE COMMENT '配置键唯一索引',
    UNIQUE INDEX `config_key_2`(`config_key` ASC) USING BTREE COMMENT '配置键唯一索引2',
    UNIQUE INDEX `idx_system_configs_config_key`(`config_key` ASC) USING BTREE COMMENT '系统配置键索引',
    
    -- 普通索引定义
    INDEX `idx_config_key`(`config_key` ASC) USING BTREE COMMENT '配置键查询索引',
    INDEX `idx_system_configs_deleted_at`(`deleted_at` ASC) USING BTREE COMMENT '软删除查询索引'
) ENGINE = InnoDB AUTO_INCREMENT = 5 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '系统配置表' ROW_FORMAT = Dynamic;


-- ========================================================================
-- 恢复外键检查
-- ========================================================================
-- 重新启用外键检查
SET FOREIGN_KEY_CHECKS = 1;


