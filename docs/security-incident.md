# 安全事件记录（2026-09-26）

## 事件摘要

服务器上的 MySQL / Redis 因**弱密码 + 端口暴露**被自动化脚本入侵：

- **MySQL**：`root@%` + 密码 `123456`，且 3306 端口对所有接口开放 → 攻击者拖库（dump 约 153KB）、清空全部表、留下勒索表 `RECOVER_YOUR_DATA_info`。
- **Redis**：无密码，6379 端口开放 → 攻击者写入 `backup1`~`backup4` 键，内容为 cron 后门载荷（从 `s.na-cs.com` 下载脚本执行）。因容器隔离（无宿主机 crontab 挂载），cron 注入未生效。
- 影响范围：仅演示数据（种子数据 + 测试数据）丢失；无 SSH 后门、无系统级持久化、未发现横向移动。拖延 30 天索要赎金的勒索话术，**不建议支付**。

## 已采取的措施

1. **端口收紧**：MySQL/Redis/Kafka/后端 API 全部只绑定 `127.0.0.1`；仅 nginx（前端入口）监听 `0.0.0.0:8080`。
2. **强密码**：MySQL root 与 Redis `requirepass` 均改为 32 位随机密码，密钥只存在服务器端 `.env`（已 gitignore，不入库）。
3. **最小权限**：删除 `root@%`，应用改用专用账号 `campus@%`（仅 `hmdp` 库的增删改查/建表权限）。
4. **数据恢复**：`db/hmdp.sql` 重新导入；被入侵的 Docker 卷全部销毁重建（mysql/redis/kafka/upload）。
5. **排查确认**：SSH authorized_keys（仅 2 把已知）、crontab、systemd、进程、容器文件系统均无后门残留。

## 仍需用户处理

1. **轮换 DeepSeek API Key**（本次未泄露，但建议轮换以绝后患）。
2. **检查 VPS 服务商 NAT 端口映射面板**：确保没有把 3306/6379/9092/8081 映射到公网；如需公网访问只开放 nginx 的 8080。
3. **谨慎恢复验证码校验开关**：当前 `login.skip_code=true`，任何能访问前端的人都能注册并调用大模型（费用风险）。对外公开前建议设回 `false`。
4. **定期备份**：`docker exec hmdp-mysql mysqldump ... > backup.sql` 或利用服务商快照。
