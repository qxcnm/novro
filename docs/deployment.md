# 部署与恢复

本文描述单机或同一内网中的生产部署。Novro 仍是一个 Go API 服务和一个 Next.js
控制台组成的模块化单体，不需要 Redis、消息队列或额外微服务。

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
```

反向代理必须透传浏览器的 `Origin` 请求头。Go 服务会对 `/api/*` 的非安全方法校验
该值是否完整匹配 `NOVRO_ALLOWED_ORIGINS`；不要用代理重写、删除或替换这个请求头。
每项来源必须是无路径、查询、片段或用户信息的 HTTPS Origin。生产环境的
`NOVRO_HTTP_ADDR` 也必须使用 loopback 主机，避免绕过反向代理直接访问 Go 服务。
`/v1/*` 的 API Key 请求不依赖浏览器 `Origin`，可由非浏览器客户端直接调用。
`NOVRO_PUBLIC_URL` 同样必须是无路径、查询、片段或用户信息的站点 Origin；OIDC
回调地址由该值拼接 `/api/auth/oidc/callback` 得到。

Next.js 进程只需要服务端变量：

```env
NOVRO_SERVER_URL=http://127.0.0.1:8080
```

不要把它改成 `NEXT_PUBLIC_*`。`NOVRO_SESSION_SECRET` 和
`NOVRO_PROVIDER_ENCRYPTION_SECRET` 必须独立生成；后者丢失后无法解密已保存的提供商
凭据。生产配置会拒绝明文数据库连接、HTTP 公共地址或非 Secure 的会话 Cookie。

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

先备份，再使用待发布版本显式执行数据库检查和迁移：

```powershell
./dist/novro.exe check-db
./dist/novro.exe migrate
```

迁移在 `ent/migrate/migrations` 中按文件名排序执行，由数据库锁防止多个实例并发迁移，
并记录到 `novro_schema_migrations`。正常服务启动不会自动修改数据库结构。

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

`-CompareSourceRowCounts` 适合源库仍在线的演练：它逐表比较 11 张业务/元数据表的精确
行数。脚本还会验证 SHA-256、完整表集合，以及恢复库中的迁移版本是否与当前仓库完全
一致。源库不可用的灾难恢复仍会执行校验和、表集合和迁移版本检查，但无法比较源行数。

恢复后使用一个只读检查账号连接目标库，抽查管理员、钱包、流水、Key 前缀、提供商和
模型路由数量；不要查询或导出密码哈希、Key 哈希或加密提供商凭据。验证完的演练库由
数据库管理员按明确库名删除。脚本不会自动删库。

## 9. 恢复切换

1. 停止所有 Novro 写流量并记录故障时间点。
2. 选择故障前最后一份校验通过的备份，恢复到新的 `novro_restore_*` 数据库。
3. 运行脚本验证和只读业务抽查；记录恢复点带来的最大数据丢失窗口。
4. 使用 `check-db` 和 `migrate` 检查恢复库，迁移只允许向前补齐。
5. 更新部署秘密中的 `NOVRO_DATABASE_NAME`，重启 Go 服务并确认 `/readyz`。
6. 恢复只读流量并完成登录、余额、Key、模型列表检查后，再恢复写流量。
7. 原数据库保持只读并按保留策略归档，不立即删除。

恢复演练不能在生产数据库名上执行，也不能使用日常应用账号授予全局建库权限。生产
备份可以由最小权限的只读备份账号完成；恢复应由短期、审计过的运维账号执行。
