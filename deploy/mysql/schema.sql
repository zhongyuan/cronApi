-- ============================================================
-- cronApi 建表语句（MySQL 8.0，utf8mb4）
--
-- 说明：
--   1. 服务启动时会自动执行 AutoMigrate 建表，本文件与之完全一致，
--      用于「不想开放 CREATE 权限」或需要 DBA 审核时手工建表。
--   2. 手工建表后，服务启动时的 AutoMigrate 检测到表已存在会直接跳过，不会冲突。
--   3. 执行方式：
--        mysql -h127.0.0.1 -P3306 -uroot -p < deploy/mysql/schema.sql
-- ============================================================

CREATE DATABASE IF NOT EXISTS `cron_api`
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_general_ci;

USE `cron_api`;

-- ------------------------------------------------------------
-- 用户表：小程序用户以 open_id 为唯一身份
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `sys_user` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `open_id` varchar(64) NOT NULL COMMENT '小程序 openid',
  `union_id` varchar(64) DEFAULT '' COMMENT '开放平台 unionid',
  `nickname` varchar(64) DEFAULT '',
  `avatar_url` varchar(512) DEFAULT '',
  `status` smallint NOT NULL DEFAULT '1' COMMENT '1 正常 0 禁用',
  `last_login` datetime(3) DEFAULT NULL COMMENT '最近登录时间',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_openid` (`open_id`),
  KEY `idx_unionid` (`union_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户';

-- ------------------------------------------------------------
-- 任务表：一条记录 = 一个定时联调任务
--   steps 字段存 JSON：{"base_url":"...","auths":{...},"steps":[...]}
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cron_task` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(128) NOT NULL,
  `owner_id` bigint DEFAULT NULL,
  `description` varchar(512) DEFAULT NULL,
  `cron_expr` varchar(64) NOT NULL COMMENT 'robfig cron 表达式(秒级可选)',
  `timezone` varchar(64) NOT NULL DEFAULT 'Asia/Shanghai',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用定时执行',
  `steps` json NOT NULL COMMENT '联调步骤(JSON)',
  `variables` json DEFAULT NULL COMMENT '默认变量(JSON)',
  `max_retry` bigint NOT NULL DEFAULT '0' COMMENT '步骤级重试次数',
  `timeout_sec` bigint NOT NULL DEFAULT '60' COMMENT '整任务超时秒数',
  `notify_url` varchar(512) DEFAULT '' COMMENT '失败回调 webhook',
  `last_run_at` datetime(3) DEFAULT NULL COMMENT '上次执行时间',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_owner` (`owner_id`),
  KEY `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='联调任务';

-- ------------------------------------------------------------
-- 执行记录表：每次运行一条，保存逐步报告
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `cron_run` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `task_id` bigint NOT NULL COMMENT '所属任务',
  `trace_id` varchar(64) DEFAULT '' COMMENT '链路 ID',
  `status` varchar(16) NOT NULL COMMENT 'running|success|failed|timeout',
  `trigger` varchar(16) NOT NULL DEFAULT 'manual' COMMENT 'schedule|manual|api',
  `total_steps` bigint NOT NULL DEFAULT '0' COMMENT '总步骤数',
  `passed_steps` bigint NOT NULL DEFAULT '0' COMMENT '通过步骤数',
  `duration_ms` bigint NOT NULL DEFAULT '0' COMMENT '总耗时(毫秒)',
  `error_msg` varchar(1024) DEFAULT '' COMMENT '失败原因',
  `steps` json DEFAULT NULL COMMENT '逐步结果明细',
  `variables` json DEFAULT NULL COMMENT '执行时的变量快照',
  `started_at` datetime(3) NOT NULL COMMENT '开始时间',
  `finished_at` datetime(3) DEFAULT NULL COMMENT '结束时间',
  PRIMARY KEY (`id`),
  KEY `idx_task_time` (`task_id`,`started_at`),
  KEY `idx_trace_id` (`trace_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='执行记录';
