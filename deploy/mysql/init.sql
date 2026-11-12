-- ============================================================
-- cronApi 在公网 MySQL 上的初始化脚本（在服务器上以 root 执行）
--   mysql -h127.0.0.1 -uroot -p < deploy/mysql/init.sql
-- ============================================================

-- 1) 建库：显式指定 utf8mb4，避免昵称/emoji 写入报错
CREATE DATABASE IF NOT EXISTS cron_api
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_general_ci;

-- 2) 建专用账号
--    主机部分决定「允许从哪里连」：
--      'cronapi'@'8.8.4.4'          仅允许 cronApi 所在服务器 IP（最安全，推荐）
--      'cronapi'@'8.8.4.0/255.255.255.0'  整个网段
--      'cronapi'@'%'                任意来源（必须配合防火墙/安全组白名单）
--    请把下面的 8.8.4.4 换成 cronApi 服务器的公网出口 IP。
CREATE USER IF NOT EXISTS 'cronapi'@'8.8.4.4' IDENTIFIED BY 'CHANGE_ME_strong_password';

-- 3) 只授予本库权限，不给全局权限
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, REFERENCES
  ON cron_api.* TO 'cronapi'@'8.8.4.4';

-- 4) 限制该账号的连接数与查询频率，防止应用异常时把数据库拖垮
ALTER USER 'cronapi'@'8.8.4.4'
  WITH MAX_USER_CONNECTIONS 60
  MAX_QUERIES_PER_HOUR 200000
  MAX_UPDATES_PER_HOUR 50000;

FLUSH PRIVILEGES;

-- 5) 自检：确认账号与授权
SELECT user, host FROM mysql.user WHERE user = 'cronapi';
SHOW GRANTS FOR 'cronapi'@'8.8.4.4';

-- 6) 建表由 cronApi 启动时自动完成（AutoMigrate），此处无需手动执行 DDL。
--    若后续关闭 AutoMigrate，可执行：mysqldump --no-data 导出结构后导入。
