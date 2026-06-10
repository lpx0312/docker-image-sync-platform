-- Docker 镜像同步平台 — Greenfield 全库 DDL
-- 用法: mysql -u user -p database < scripts/init.sql
-- 说明: 本文件为唯一权威 schema；应用 AutoMigrate 仅做 seed，不建同步三表

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ---------------------------------------------------------------------------
-- RBAC
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `permissions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `code` VARCHAR(50) NOT NULL COMMENT '权限代码，如 sync、images',
  `name` VARCHAR(100) NOT NULL COMMENT '权限显示名称',
  `description` VARCHAR(255) DEFAULT NULL COMMENT '权限说明',
  `sort_order` INT NOT NULL DEFAULT 0 COMMENT '排序序号',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_permissions_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='权限定义表';

CREATE TABLE IF NOT EXISTS `roles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `code` VARCHAR(50) NOT NULL COMMENT '角色代码，如 admin、operator',
  `name` VARCHAR(100) NOT NULL COMMENT '角色名称',
  `description` VARCHAR(255) DEFAULT NULL COMMENT '角色说明',
  `is_system` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否系统内置角色',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_roles_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色表';

CREATE TABLE IF NOT EXISTS `role_permissions` (
  `role_id` BIGINT UNSIGNED NOT NULL COMMENT '角色 ID',
  `permission_id` BIGINT UNSIGNED NOT NULL COMMENT '权限 ID',
  PRIMARY KEY (`role_id`, `permission_id`),
  KEY `idx_role_permissions_permission_id` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色与权限关联表';

CREATE TABLE IF NOT EXISTS `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `username` VARCHAR(50) NOT NULL COMMENT '登录用户名',
  `password_hash` VARCHAR(255) NOT NULL COMMENT '密码哈希',
  `email` VARCHAR(100) DEFAULT NULL COMMENT '邮箱',
  `role_id` BIGINT UNSIGNED NOT NULL COMMENT '角色 ID',
  `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '账号状态：active/disabled',
  `last_login_at` DATETIME(3) DEFAULT NULL COMMENT '最后登录时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_users_username` (`username`),
  KEY `idx_users_role_id` (`role_id`),
  KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户账号表';

CREATE TABLE IF NOT EXISTS `login_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户 ID',
  `username` VARCHAR(50) NOT NULL COMMENT '登录用户名快照',
  `ip` VARCHAR(45) DEFAULT NULL COMMENT '客户端 IP',
  `user_agent` VARCHAR(500) DEFAULT NULL COMMENT 'User-Agent',
  `status` VARCHAR(20) NOT NULL COMMENT '登录结果：success/failed',
  `message` VARCHAR(255) DEFAULT NULL COMMENT '结果说明',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '记录时间',
  PRIMARY KEY (`id`),
  KEY `idx_login_logs_user_id` (`user_id`),
  KEY `idx_login_logs_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户登录日志表';

-- ---------------------------------------------------------------------------
-- 系统配置（仅 Git 等，不含 ACR 凭据）
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `system_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `config_key` VARCHAR(100) NOT NULL COMMENT '配置键',
  `config_value` TEXT COMMENT '配置值',
  `description` VARCHAR(500) DEFAULT NULL COMMENT '配置说明',
  `is_encrypted` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否加密存储',
  `config_group` VARCHAR(50) NOT NULL DEFAULT 'default' COMMENT '配置分组',
  `display_order` INT NOT NULL DEFAULT 0 COMMENT '展示排序',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_system_configs_key` (`config_key`),
  KEY `idx_system_configs_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统配置表（Git 等）';

-- ---------------------------------------------------------------------------
-- ACR
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `acr_registries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `registry_url` VARCHAR(255) NOT NULL COMMENT '镜像仓库地址',
  `namespace` VARCHAR(100) NOT NULL COMMENT '命名空间',
  `username` VARCHAR(100) NOT NULL COMMENT '仓库用户名',
  `password` VARCHAR(500) NOT NULL COMMENT '仓库密码（加密）',
  `auth_server` VARCHAR(255) DEFAULT NULL COMMENT 'Docker 认证服务器地址',
  `docker_service` VARCHAR(255) DEFAULT NULL COMMENT 'Docker Registry Service 标识',
  `is_default` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为默认 ACR',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_acr_registries_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ACR 实例配置表';

CREATE TABLE IF NOT EXISTS `acr_repositories` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `acr_registry_id` BIGINT UNSIGNED NOT NULL COMMENT '所属 ACR ID',
  `repository_name` VARCHAR(255) NOT NULL COMMENT '仓库名（不含 namespace）',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_acr_repositories_registry` (`acr_registry_id`),
  KEY `idx_acr_repositories_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ACR 镜像仓库台账表';

-- ---------------------------------------------------------------------------
-- 同步三表
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `sync_batches` (
  `id` CHAR(36) NOT NULL COMMENT '批次 UUID，对外亦称 task_id',
  `description` VARCHAR(500) DEFAULT NULL COMMENT '用户备注',
  `is_mock` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否模拟同步批次',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_sync_batches_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='同步批次头表（状态由明细聚合）';

CREATE TABLE IF NOT EXISTS `sync_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `batch_id` CHAR(36) NOT NULL COMMENT '所属批次 ID',
  `acr_registry_id` BIGINT UNSIGNED NOT NULL COMMENT '目标 ACR ID',
  `original_image` VARCHAR(500) NOT NULL COMMENT '源镜像名',
  `acr_image` VARCHAR(500) DEFAULT NULL COMMENT '同步后的 ACR 完整地址',
  `tag` VARCHAR(100) NOT NULL DEFAULT 'latest' COMMENT '镜像标签',
  `architecture` VARCHAR(50) NOT NULL DEFAULT '' COMMENT '用户指定架构（可选）',
  `acr_architectures` TEXT COMMENT 'ACR 实测架构 JSON 数组',
  `original_input` VARCHAR(600) DEFAULT NULL COMMENT '用户原始输入行',
  `input_order` INT NOT NULL DEFAULT 0 COMMENT '批次内输入顺序',
  `status` ENUM('pending','syncing','success','failed') NOT NULL DEFAULT 'pending' COMMENT '同步状态（API 字段 sync_status）',
  `error_message` TEXT COMMENT '失败原因',
  `description` VARCHAR(500) DEFAULT NULL COMMENT '单镜像说明',
  `retry_count` INT NOT NULL DEFAULT 0 COMMENT '已重试次数',
  `started_at` DATETIME(3) DEFAULT NULL COMMENT '开始同步时间',
  `completed_at` DATETIME(3) DEFAULT NULL COMMENT '完成时间',
  `duration` BIGINT NOT NULL DEFAULT 0 COMMENT '耗时（秒）',
  `image_size` BIGINT NOT NULL DEFAULT 0 COMMENT '镜像大小（字节，预留）',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_sync_records_batch_id` (`batch_id`),
  KEY `idx_sync_records_acr_registry_id` (`acr_registry_id`),
  KEY `idx_sync_records_status` (`status`),
  KEY `idx_sync_records_original_image` (`original_image`),
  KEY `idx_sync_records_input_order` (`input_order`),
  KEY `idx_sync_records_created_at` (`created_at`),
  KEY `idx_sync_records_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='镜像同步明细表';

CREATE TABLE IF NOT EXISTS `sync_workflow_runs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `batch_id` CHAR(36) NOT NULL COMMENT '批次 ID',
  `acr_registry_id` BIGINT UNSIGNED NOT NULL COMMENT 'ACR ID',
  `github_run_id` VARCHAR(100) DEFAULT NULL COMMENT 'GitHub Actions Run ID',
  `github_action_url` VARCHAR(500) DEFAULT NULL COMMENT 'GitHub Actions 运行页 URL',
  `commit_sha` VARCHAR(100) DEFAULT NULL COMMENT '触发提交 SHA',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sync_workflow_runs_batch_acr` (`batch_id`, `acr_registry_id`),
  KEY `idx_sync_workflow_runs_batch_id` (`batch_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='批次×ACR 的 GitHub Actions 运行记录';

SET FOREIGN_KEY_CHECKS = 1;
