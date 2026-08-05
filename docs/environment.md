# 开发环境记录

记录日期：2026-08-05

## 已准备环境

| 环境 | 版本 | 状态 | 位置 |
| --- | --- | --- | --- |
| Go | 1.26.5 | 已下载并可执行 | `C:\Users\qxnm\tools\go1.26.5` |
| Node.js | 24.18.0 | 已存在 | `C:\nvm4w\nodejs` |
| pnpm | 10.30.3 | 已存在 | `C:\nvm4w\nodejs` |
| MySQL | 云端实例 | 应用直连与首个迁移已验证 | `117.72.17.16:3306` |

Go 用户级 `bin` 目录已加入当前用户 PATH。新开的终端可以直接执行 `go version`；当前会话也可以直接使用完整路径验证 Go。

## 云端 MySQL

第一版使用云端 MySQL，不依赖本机 MySQL 服务。连接信息如下：

```text
Host: 117.72.17.16
Port: 3306
Database: novro-db
```

管理员账号只用于首次创建数据库和应用账号，不作为 Novro 的运行账号。管理员密码不得写入 Markdown、源码、日志或提交到 Git。由于管理员密码已经通过聊天发送，正式使用前需要更换。

应用运行时使用独立的 `novro_app` 用户，并通过部署环境变量注入密码。MySQL 的 `3306` 端口只允许开发机和应用服务器的固定公网 IP 访问，不允许向整个公网开放。

建议由数据库管理员首次执行：

```sql
CREATE DATABASE `novro-db`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE USER 'novro_app'@'<APP_SERVER_IP>'
  IDENTIFIED BY '<GENERATED_STRONG_PASSWORD>';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP
  ON `novro-db`.* TO 'novro_app'@'<APP_SERVER_IP>';

FLUSH PRIVILEGES;
```

如果开发机和正式应用服务器的公网 IP 不同，分别创建受来源 IP 限制的账号。不要使用 `'novro_app'@'%'`。

计划使用的环境变量：

```env
NOVRO_DATABASE_DRIVER=mysql
NOVRO_DATABASE_HOST=117.72.17.16
NOVRO_DATABASE_PORT=3306
NOVRO_DATABASE_NAME=novro-db
NOVRO_DATABASE_USER=novro_app
NOVRO_DATABASE_PASSWORD=<LOCAL_OR_DEPLOYMENT_SECRET>
NOVRO_DATABASE_TLS=true
```

真实 `.env` 文件需要加入 `.gitignore`。仓库只允许保留不含真实密码的 `.env.example`。

## 当前验证记录

2026-08-05 已确认 `117.72.17.16:3306` 可达，用户也已通过 DBeaver 连接。实例证书
不在本机信任链中，因此应用采用与该客户端一致的方式：保持 TLS 加密但不验证
证书链。配置不包含证书校验开关，也不会在 TLS 开启时降级到明文连接。

同日已通过仅注入进程环境的管理员凭据执行 `init-db`，创建 `novro-db`，随后完成
连接检查并应用 `0001_users_and_sessions` 迁移。真实密码没有写入文件或日志。

Next.js、TypeScript、原始 shadcn/ui、Ent 和 MySQL 驱动已经初始化。应用账号与
按开发启动说明显式执行连接检查和迁移。
