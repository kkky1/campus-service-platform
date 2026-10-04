#!/bin/sh
# MySQL 首次初始化：创建应用专用最小权限账号（对齐 compose 方案）。
# 注意：MYSQL_DATABASE=hmdp 已由 mysql 镜像自动创建；此处仅补 app 账号。
set -e

mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" <<-EOSQL
  CREATE DATABASE IF NOT EXISTS hmdp CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  CREATE USER IF NOT EXISTS 'campus'@'%' IDENTIFIED BY '${MYSQL_APP_PASSWORD}';
  GRANT ALL PRIVILEGES ON hmdp.* TO 'campus'@'%';
  FLUSH PRIVILEGES;
EOSQL
