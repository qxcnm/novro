# 开发环境记录

记录日期：2026-08-05

## 已准备环境

| 环境 | 版本 | 状态 | 位置 |
| --- | --- | --- | --- |
| Go | 1.26.5 | 已下载并可执行 | `C:\Users\qxnm\tools\go1.26.5` |
| Node.js | 24.18.0 | 已存在 | `C:\nvm4w\nodejs` |
| pnpm | 10.30.3 | 已存在 | `C:\nvm4w\nodejs` |
| MySQL | 云端 `8.0.46` | 应用账号与 TLS 早期验证；当前代码使用单个初始化迁移 | `<DATABASE_HOST>:3306` |

Go 用户级 `bin` 目录已加入当前用户 PATH。新开的终端可以直接执行 `go version`；当前会话也可以直接使用完整路径验证 Go。

## 云端 MySQL

第一版使用云端 MySQL，不依赖本机 MySQL 服务。连接信息如下：

```text
Host: <DATABASE_HOST>
Port: 3306
Database: novro
```

管理员账号只用于首次创建数据库和应用账号，不作为 Novro 的运行账号。管理员密码不得写入 Markdown、源码、日志或提交到 Git。由于管理员密码已经通过聊天发送，正式使用前需要更换。

应用运行时使用独立的 `novro_app` 用户，并通过部署环境变量注入密码。MySQL 的 `3306` 端口只允许开发机和应用服务器的固定公网 IP 访问，不允许向整个公网开放。

数据库管理员已为当前开发来源创建受限账号。其他部署来源按同样边界执行：

```sql
CREATE DATABASE `novro`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE USER 'novro_app'@'<APP_SERVER_IP>'
  IDENTIFIED BY '<GENERATED_STRONG_PASSWORD>'
  REQUIRE SSL;

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, REFERENCES,
  CREATE TEMPORARY TABLES, LOCK TABLES
  ON `novro`.* TO 'novro_app'@'<APP_SERVER_IP>';

FLUSH PRIVILEGES;
```

如果开发机和正式应用服务器的公网 IP 不同，分别创建受来源 IP 限制的账号。不要使用 `'novro_app'@'%'`。

计划使用的环境变量：

```env
NOVRO_DATABASE_DRIVER=mysql
NOVRO_DATABASE_HOST=<DATABASE_HOST>
NOVRO_DATABASE_PORT=3306
NOVRO_DATABASE_NAME=novro
NOVRO_DATABASE_USER=novro_app
NOVRO_DATABASE_PASSWORD=<LOCAL_OR_DEPLOYMENT_SECRET>
NOVRO_DATABASE_TLS=true
```

真实 `.env` 文件需要加入 `.gitignore`。仓库只允许保留不含真实密码的 `.env.example`。

## 当前验证记录

2026-08-05 已确认 `<DATABASE_HOST>:3306` 可达，用户也已通过 DBeaver 连接。实例证书
不在本机信任链中，因此应用采用与该客户端一致的方式：保持 TLS 加密但不验证
证书链。配置不包含证书校验开关，也不会在 TLS 开启时降级到明文连接。

同日已通过仅注入进程环境的管理员凭据执行 `init-db` 并完成连接检查。2026-08-06 的旧云库
验证基于当时的增量迁移链，当前代码已按重新部署空库压缩为 `0001_initial_schema.sql`。
正式部署当前版本时应重新创建空库并显式执行 `migrate`，不要把旧云库的迁移记录与当前
squash 后的迁移文件混用。返现比例保存在数据库并由 `/admin/referral` 管理；
`NOVRO_REFERRAL_REWARD_BPS` 只作为数据库无记录时的默认值。开发环境在数据库和环境变量都
没有配置 SMTP 时，验证码只写入 Go 服务结构化日志；生产环境未配置时不会记录验证码。
`NOVRO_EMAIL_SMTP_*` 与 `NOVRO_EMAIL_FROM` 只作为首次启动兜底，管理员在 `/admin/email`
保存后数据库配置优先并立即生效。SMTP 密码使用与提供商凭据相同的 AES-256-GCM 密钥边界加密保存。

同日通过当前 Go 服务和 Next.js 控制台完成云库端到端验证：注册与登录、余额读取、
API Key 一次性展示和复制、`/v1/models` 鉴权、管理员用户/Key/提供商/模型路由页面、
提供商凭据密文存储、明确失败上游请求的等额预占退款，以及 Key 撤销后的即时 `401`。
验收创建的用户、Key、提供商、模型路由和钱包流水随后在限定事务中清理，云库恢复为
原有两个用户、两个钱包且无测试 Key、提供商、模型路由或流水。

Next.js、TypeScript、原始 shadcn/ui、Ent 和 MySQL 驱动已经初始化。应用账号与
按开发启动说明显式执行连接检查和迁移。
