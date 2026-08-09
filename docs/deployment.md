# 部署与恢复

本文描述单机或同一内网中的生产部署。Novro 仍是一个 Go API 服务和一个 Next.js
控制台组成的模块化单体，不需要 Redis、消息队列或额外微服务。

如果希望只维护一个对外的 Novro 应用服务，优先使用 [Docker 单应用部署](docker-deployment.md)：
Nginx、Go 和 Next.js 在同一个应用容器内运行，MySQL 作为内部数据库容器运行，脚本会
完成环境安装、迁移、初始化和就绪检查。本文后续章节保留源码进程部署、备份和恢复细节。

## 1. 前置条件

- Windows Server 或 Linux x86-64 主机，安装 Go `1.26.5`、Node.js `24` 和 pnpm `10.30.3`
- MySQL `8.4`，应用使用来源 IP 受限的专用账号，不使用 `root`
- MySQL `8.4` 客户端工具 `mysql` 和 `mysqldump`
- 能终止 TLS 的反向代理，并具有可续期的受信任 HTTPS 证书
- 由部署平台或系统服务管理器保存环境变量和秘密

当前云数据库地址为 `117.72.17.16:3306`，数据库名为 `novro-db`。只允许部署主机和
指定运维主机的固定公网 IP 访问，不要将 `3306` 对整个公网开放。

## 2. 生产配置

从 `.env.example` 复制变量名到部署平台的秘密存储。生产环境至少设置：

```env
NOVRO_ENVIRONMENT=production
NOVRO_HTTP_ADDR=127.0.0.1:8080

NOVRO_DATABASE_DRIVER=mysql
NOVRO_DATABASE_HOST=117.72.17.16
NOVRO_DATABASE_PORT=3306
NOVRO_DATABASE_NAME=novro-db
NOVRO_DATABASE_USER=novro_app
NOVRO_DATABASE_PASSWORD=<DEPLOYMENT_SECRET>
NOVRO_DATABASE_TLS=true

NOVRO_SESSION_SECRET=<AT_LEAST_32_RANDOM_BYTES>
NOVRO_PROVIDER_ENCRYPTION_SECRET=<SEPARATE_AT_LEAST_32_BYTE_SECRET>
NOVRO_SESSION_COOKIE_SECURE=true
NOVRO_PUBLIC_URL=https://novro.example.com
NOVRO_ALLOWED_ORIGINS=https://novro.example.com

# Optional first-run email fallback. The admin email page becomes the runtime
# source of truth after the first save.
NOVRO_EMAIL_SMTP_HOST=smtp.example.com
NOVRO_EMAIL_SMTP_PORT=587
NOVRO_EMAIL_SMTP_USERNAME=novro@example.com
NOVRO_EMAIL_SMTP_PASSWORD=<DEPLOYMENT_SECRET>
NOVRO_EMAIL_SMTP_TLS=true
NOVRO_EMAIL_FROM=novro@example.com

# Optional first-run bootstrap defaults. The admin payment page becomes the
# runtime source of truth after the first save.
NOVRO_EPAY_API_URL=https://pay.example.com
NOVRO_EPAY_MERCHANT_ID=<EPAY_MERCHANT_ID>
NOVRO_EPAY_MERCHANT_KEY=<EPAY_MERCHANT_SECRET>
NOVRO_EPAY_SITE_NAME=Novro
NOVRO_EPAY_CHANNELS=alipay,wxpay
```

反向代理必须透传浏览器的 `Origin` 请求头。Go 服务会对 `/api/*` 的非安全方法校验
该值是否完整匹配 `NOVRO_ALLOWED_ORIGINS`；不要用代理重写、删除或替换这个请求头。
每项来源必须是无路径、查询、片段或用户信息的 HTTPS Origin。生产环境的
`NOVRO_HTTP_ADDR` 也必须使用 loopback 主机，避免绕过反向代理直接访问 Go 服务。
`/v1/*` 的 API Key 请求不依赖浏览器 `Origin`，可由非浏览器客户端直接调用。
`NOVRO_PUBLIC_URL` 同样必须是无路径、查询、片段或用户信息的站点 Origin；OIDC
回调地址由该值拼接 `/api/auth/oidc/callback` 得到，易支付异步通知与同步返回地址分别由
该值拼接 `/api/payments/epay/notify` 和 `/api/payments/epay/return` 得到。反向代理必须把这两个
地址转发到 Go 服务；同步返回会验签、幂等入账后再跳转到余额页。

支付平台已经成功但通知未到达时，不要直接修改钱包或删除、重建订单。先从支付平台账单确认
Novro 订单号，再在使用同一生产配置的运维环境执行：

```powershell
./dist/novro.exe reconcile-top-up <NOVRO_OUT_TRADE_NO>
```

命令只读查询支付平台；仅当平台返回已支付且订单号、金额、渠道全部匹配时，才复用通知处理的
事务完成入账。已到账订单会直接返回当前状态，重复执行不会重复增加余额或新增充值流水。
登录用户也可以在余额页对自己的待支付订单点击“查询支付结果”；该操作使用相同的核验和入账
事务，不能读取其他用户的订单，不会重新发起支付、创建订单或删除历史记录。

Next.js 进程只需要服务端变量：

```env
NOVRO_SERVER_URL=http://127.0.0.1:8080
```

该值必须是无凭据、路径、查询或片段的 `http/https` Origin；生产构建只接受
`localhost`、127/8 或 `::1` 回环地址，并应在构建和启动 Next.js 时设置同一个值。
不要把它改成 `NEXT_PUBLIC_*`。公开注册依赖 SMTP 发送一次性验证码。部署可通过上述环境变量
提供首次兜底，也可在管理员登录后通过 `/admin/email` 保存配置；数据库配置保存后立即生效并优先于
环境变量。生产环境没有完整 SMTP 配置时服务仍可启动，但验证码发送会返回不可用，不会把验证码写入日志。
SMTP 密码只能存放在服务端秘密存储或管理页面中，管理页面使用 AES-256-GCM 加密后入库。
`NOVRO_SESSION_SECRET` 和
`NOVRO_PROVIDER_ENCRYPTION_SECRET` 必须独立生成；后者丢失后无法解密已保存的提供商
凭据和支付商户密钥。`NOVRO_EPAY_MERCHANT_KEY` 也是服务端秘密，不得写入 `NEXT_PUBLIC_*`、前端配置或
日志；三个支付凭据变量可以一起留空，管理员可在 `/admin/payments` 完成配置。生产配置会拒绝
明文数据库连接、HTTP 支付网关、HTTP 公共地址或非 Secure 的会话 Cookie。

## 3. 构建与发布目录

在干净的发布工作区执行：

```powershell
pnpm install --frozen-lockfile
go mod download
go test ./cmd/... ./internal/... ./ent/...
go vet ./...
pnpm --dir apps/web lint
pnpm --dir apps/web typecheck
pnpm --dir apps/web test
go build -trimpath -o dist/novro.exe ./cmd/novro
pnpm --dir apps/web build
```

发布物包括 `dist/novro.exe`、`apps/web/.next`、`apps/web/public`、根目录
`package.json`/`pnpm-lock.yaml` 和生产依赖。不要发布 `.env`、日志、测试截图、`tmp/` 或
`backups/`。

## 4. 迁移与启动

当前发布包只保留 `0001_initial_schema.sql`，用于重新部署时初始化全新空库。不要把保留旧
增量迁移记录的数据库直接交给当前迁移器；需要保留旧数据时，应先使用与旧库匹配的历史
版本完成升级或另行编写数据迁移。本次重新部署应创建空库，再执行数据库检查和迁移：

```powershell
./dist/novro.exe check-db
./dist/novro.exe migrate
```

迁移在 `ent/migrate/migrations` 中按文件名排序执行，由数据库锁防止多个实例并发迁移，
并把版本和 SQL 的 SHA-256 记录到 `novro_schema_migrations`。初始化完成后，后续新增迁移
仍会按顺序前向执行；历史 SQL 被修改，或发布包缺失数据库中已应用的迁移时，迁移命令会
拒绝继续。正常服务启动不修改数据库结构，发布流程必须在启动新版本前显式执行并确认
迁移成功。

启动两个受进程管理器监督的进程：

```powershell
./dist/novro.exe
pnpm --dir apps/web start --hostname 127.0.0.1 --port 3000
```

Go 进程监听 `127.0.0.1:8080`，Next.js 监听 `127.0.0.1:3000`。进程管理器应在异常
退出时重启、限制日志保留时间，并在停机时给 Go 服务至少 10 秒完成优雅关闭。

## 5. 反向代理与 TLS

公网只开放 `443`。反向代理将所有请求转发给 `127.0.0.1:3000`；Next.js 会把
`/api/*` 转发给 Go 服务。模型兼容接口 `/v1/*` 应直接转发给 `127.0.0.1:8080`，避免
流式响应经过不必要的一层代理。SSE 路径必须关闭响应缓冲并使用足够长的读取超时。

代理需要保留 `Host`，设置 `X-Forwarded-Proto=https`，并禁止访问内部监听端口。
证书在反向代理终止；浏览器 Cookie 始终使用 Secure。不要在代理访问日志中记录
`Authorization`、Cookie、请求正文或上游 API Key。

## 6. 健康检查与上线

本机检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
Invoke-WebRequest http://127.0.0.1:3000/login -UseBasicParsing
```

`/healthz` 仅表示进程存活；`/readyz` 在两秒超时内检查数据库。只有两个 API 检查和
登录页都通过后，才将反向代理流量切到新版本。切换后从公网 HTTPS 地址检查代理实际
暴露的健康检查路径、登录、控制台导航、API Key 创建/复制和一个受控的 `/v1/models`
请求。若公网需要 `/healthz` 和 `/readyz`，反向代理必须把这两个精确路径直接转发给
Go 服务；否则只在部署主机本地检查它们。

上线顺序：备份、构建、停止旧版本写流量、迁移、启动新版本、就绪检查、切换流量、
冒烟测试。迁移是前向执行的，不支持自动回滚 SQL。

网关会对结算和失败退款的短暂存储错误做有限幂等重试。日志出现
`finalize gateway usage` 或 `refund gateway reservation` 且重试耗尽时，使用日志中的
`request_id` 对照提供商请求记录、`api_usages` 和钱包流水人工核对。上游已经成功的
请求不能因为本地结算失败就自动退款；确认结果后再通过管理员余额调整修正，并保留
对账记录。

应用回滚只切回与当前数据库结构兼容的上一版本。若迁移导致数据或结构不兼容，应保持
写流量停止，从已验证备份恢复到新的数据库，再修改 `NOVRO_DATABASE_NAME` 指向恢复库，
检查就绪后切换；不要在原生产库上直接覆盖恢复。

## 7. 备份

备份脚本读取现有 `NOVRO_DATABASE_*` 环境变量，密码通过临时 MySQL option file 传给
客户端，不会出现在命令参数中。输出先写入 `.partial`，成功后原子改名，并生成
SHA-256 校验文件。

```powershell
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
./scripts/mysql-backup.ps1 -OutputPath "./backups/novro-$stamp.sql"
```

将 `.sql` 和 `.sha256` 一起复制到加密、访问受限、与数据库主机故障域不同的存储。
备份成功不等于可恢复；至少每月执行一次隔离恢复演练，并保留日期、备份哈希、表数、
迁移数、行数比较结果和操作者记录。

生产备份账号应与 `novro_app` 分开并限制来源 IP。数据库管理员只授予备份所需的
`SELECT`、`SHOW VIEW`、`TRIGGER`、`EVENT` 和当前 MySQL 版本读取存储过程元数据所需
的最小权限；不要把全局建库或写入权限授予日常备份账号。执行脚本的单独运维进程可用
同名 `NOVRO_DATABASE_*` 变量临时注入该账号，应用服务本身仍使用 `novro_app`。

## 8. 隔离恢复演练

恢复账号需要在目标 MySQL 实例创建数据库的权限。目标库默认必须以
`novro_restore_` 开头，且脚本拒绝目标与 `NOVRO_DATABASE_NAME` 相同或目标库已经存在。

```powershell
$target = 'novro_restore_' + (Get-Date -Format 'yyyyMMdd_HHmmss')
./scripts/mysql-restore.ps1 `
  -BackupPath './backups/novro-20260806-080000.sql' `
  -TargetDatabase $target `
  -CompareSourceRowCounts
```

`-CompareSourceRowCounts` 适合源库仍在线的演练：它逐表比较恢复点所含业务/元数据表的
精确行数。脚本默认要求并验证备份的 SHA-256 文件，再检查表集合与迁移记录一致；已
应用的迁移必须是当前仓库的连续前缀，若元数据已有迁移 checksum，还会逐条与仓库 SQL
比对。恢复点必须来自当前单一初始化迁移基线；它可以缺少该基线之后尚未应用的后续迁移，
第 9 节的 `migrate` 会向前补齐。同一基线的旧备份若
尚无迁移 checksum 列，脚本会明确返回 `MigrationChecksumsVerified=False`，恢复后必须
先运行 `migrate` 建立校验和基线。只有经过审查的灾难恢复才能显式使用
`-AllowMissingBackupChecksum` 跳过缺失的备份校验文件，并应记录原因。源库不可用时
无法比较行数，其余检查仍会执行。

恢复后使用一个只读检查账号连接目标库，抽查管理员、钱包、流水、Key 前缀、提供商和
模型路由数量；不要查询或导出密码哈希、Key 哈希或加密提供商凭据。验证完的演练库由
数据库管理员按明确库名删除。脚本不会自动删库。

## 9. 恢复切换

1. 停止所有 Novro 写流量并记录故障时间点。
2. 选择故障前最后一份校验通过的备份，恢复到新的 `novro_restore_*` 数据库。
3. 运行脚本验证和只读业务抽查；记录恢复点带来的最大数据丢失窗口及待补迁移数。
4. 使用 `check-db` 和 `migrate` 检查恢复库，迁移只允许在当前初始化基线上向前补齐；
   缺少 checksum 的同基线旧备份在此建立校验和基线。
5. 更新部署秘密中的 `NOVRO_DATABASE_NAME`，重启 Go 服务并确认 `/readyz`。
6. 恢复只读流量并完成登录、余额、Key、模型列表检查后，再恢复写流量。
7. 原数据库保持只读并按保留策略归档，不立即删除。

恢复演练不能在生产数据库名上执行，也不能使用日常应用账号授予全局建库权限。生产
备份可以由最小权限的只读备份账号完成；恢复应由短期、审计过的运维账号执行。
