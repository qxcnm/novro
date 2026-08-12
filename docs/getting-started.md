# 开发启动说明

本文只覆盖本地开发。生产变量、自动安装 Docker/MySQL、初始化、反向代理、上线、回滚、
备份和恢复流程见 [生产部署手册](docker-deployment.md)。

## 1. 前置条件

- Go `1.26.5`
- Node.js `24` 与 pnpm `10.30.3`
- MySQL `8.4`，运行账号必须是最小权限的专用账号
- 可访问的云端 MySQL 实例

复制 `.env.example` 中的变量名到本机安全环境。不要把真实 `.env`、数据库密码、
会话密钥或 CA 私钥提交到 Git。前端仅使用服务端变量 `NOVRO_SERVER_URL`，默认
连接 `http://127.0.0.1:8080`；它必须是无凭据、路径、查询或片段的 `http/https`
Origin，且不得使用 `NEXT_PUBLIC_` 前缀。

## 2. 安装与检查

```powershell
pnpm install
go mod download
make check
```

Windows 没有 `make` 时可分别执行：

```powershell
go test ./cmd/... ./internal/... ./ent/...
go vet ./cmd/... ./internal/... ./ent/...
pnpm --dir apps/web lint
pnpm --dir apps/web typecheck
pnpm --dir apps/web test
pnpm --dir apps/web build
```

## 3. 数据库与迁移

数据库名以部署环境为准，当前开发实例使用 `novro`。应用运行时不要使用
`root`。当前实例直接设置：

```env
NOVRO_DATABASE_TLS=true
```

当前 MySQL 客户端保持 TLS 加密但不验证证书链，与现有 DBeaver 连接方式一致；
`NOVRO_DATABASE_TLS=false` 只允许用于明确受控的本地开发环境。
首次使用管理员账号创建数据库，然后切换到应用账号验证连接。可以在启动前显式执行迁移：

```powershell
go run ./cmd/novro init-db
go run ./cmd/novro check-db
go run ./cmd/novro migrate
```

`init-db` 只创建配置中的数据库，不创建表。启动服务前必须显式执行 `migrate`；
正常服务启动不会修改数据库结构。

迁移位于 `ent/migrate/migrations`，需要与 Ent Schema 一起审查。`migrate` 按文件名顺序
执行迁移，由数据库锁避免并发执行，并核对已执行文件的 SHA-256。

## 4. 初始化管理员

迁移完成后，推荐生成一次性初始化令牌并启动服务：

```powershell
$env:NOVRO_SETUP_TOKEN='<GENERATED_RANDOM_VALUE>'
go run ./cmd/novro
```

访问 `http://localhost:3000/setup`，输入该令牌并创建管理员。成功后从部署环境删除
`NOVRO_SETUP_TOKEN`。数据库会原子地关闭初始化入口，不能通过重复请求创建第二个
初始管理员。源码命令默认使用 `novro` 作为账号；密码必须显式通过环境变量提供，首次登录后应
立即重置，生产环境不要长期保留初始化密码。

无浏览器环境仍可通过当前进程环境变量创建第一个管理员：

```powershell
$env:NOVRO_BOOTSTRAP_USERNAME='novro'
$env:NOVRO_BOOTSTRAP_EMAIL='novro@example.invalid'
$env:NOVRO_BOOTSTRAP_DISPLAY_NAME='Novro'
$env:NOVRO_BOOTSTRAP_PASSWORD='<SET_A_RANDOM_PASSWORD>'
go run ./cmd/novro bootstrap-admin
Remove-Item Env:NOVRO_BOOTSTRAP_PASSWORD
```

用户名、邮箱和密码都是必填项，密码必须为 8 到 72 字节，且至少包含一个英文字符和一个数字。
命令不接受明文密码参数，也不会打印密码。

首次引导的 `novro` 会被标记为系统管理员，不能被停用或降级。遗忘密码时可在维护窗口执行：

```powershell
$env:NOVRO_ADMIN_PASSWORD='<SET_A_RANDOM_PASSWORD>'
go run ./cmd/novro reset-admin
Remove-Item Env:NOVRO_ADMIN_PASSWORD
```

生产环境应通过容器维护命令临时注入该变量，命令完成后立即移除，不要把密码写入 Compose 文件或镜像。

## 5. 注册与 OIDC

`NOVRO_REGISTRATION_ENABLED=true` 允许公开注册普通成员，不能注册管理员。首次
管理员初始化完成前，注册和 OIDC 自动注册都会被拒绝。本地注册需要用户名、邮箱、密码和邮箱验证码；
登录时可填写用户名或邮箱。邀请链接会在注册请求中附带一次性的邀请关系：

```env
NOVRO_REFERRAL_REWARD_BPS=1000
```

该值使用基点，`1000` 表示好友充值确认后返现 10%，`0` 表示停止产生新的邀请返现。
初始化迁移会写入数据库默认值；管理员在 `/admin/referral` 保存的数据库设置优先，修改后无需重启服务。

管理员可在 `/admin/email` 配置 SMTP 主机、端口、传输加密、发件身份和凭据，并发送测试邮件。
页面保存后配置立即生效，SMTP 密码只以 AES-256-GCM 密文保存，读取接口不会返回密码。
开发环境完全没有保存配置和环境变量兜底时，验证码只写入 Go 服务结构化日志；生产环境未完成配置时不会写日志，也不会发送验证码。

以下环境变量仅作为首次启动兜底。数据库中保存过配置后，以管理页面为准：

```env
NOVRO_EMAIL_SMTP_HOST=smtp.example.com
NOVRO_EMAIL_SMTP_PORT=587
NOVRO_EMAIL_SMTP_USERNAME=novro@example.com
NOVRO_EMAIL_SMTP_PASSWORD=<DEPLOYMENT_SECRET>
NOVRO_EMAIL_SMTP_TLS=true
NOVRO_EMAIL_FROM=novro@example.com
```

环境变量兜底目前使用 STARTTLS。管理页面还支持 SSL/TLS（通常为 465 端口）和无加密；正式环境应使用 STARTTLS 或 SSL/TLS。

配置第三方 OIDC 时，在身份平台登记回调地址：

```text
http://localhost:3000/api/auth/oidc/callback
```

生产环境应改为 `NOVRO_PUBLIC_URL` 对应的 HTTPS 域名。OIDC 配置示例：

```env
NOVRO_PUBLIC_URL=https://YOUR_DOMAIN
NOVRO_OIDC_ISSUER=https://id.example.com
NOVRO_OIDC_CLIENT_ID=novro
NOVRO_OIDC_CLIENT_SECRET=<DEPLOYMENT_SECRET>
NOVRO_OIDC_DISPLAY_NAME=企业账号
NOVRO_OIDC_AUTO_REGISTER=true
```

Issuer 必须支持 OIDC Discovery，且 Issuer、Client ID、Client Secret 必须同时配置；
缺少任意一项时 OIDC 不会启动。Client Secret 只放在服务端部署环境，不使用
`NEXT_PUBLIC_*`。设置 `NOVRO_OIDC_AUTO_REGISTER=false` 后，只有已绑定的外部身份
可以登录。

## 6. 启动

分别启动 Go 服务和控制台：

```powershell
go run ./cmd/novro
pnpm --dir apps/web dev
```

- 控制台：`http://localhost:3000`
- API 接入文档：`http://localhost:3000/docs`
- 模型与官方价格：`http://localhost:3000/models`
- 存活检查：`http://localhost:8080/healthz`
- 就绪检查：`http://localhost:8080/readyz`

`/healthz` 只表示进程可服务；`/readyz` 会在两秒超时内检查数据库，但不会把
连接错误返回给客户端。

## 7. 当前功能

- 用户名或邮箱登录、退出和当前用户查询
- 管理员用户列表、搜索、筛选、分页、创建、资料与角色编辑、启用、停用和密码重置
- 最后一个启用管理员不能被停用或降级
- 重置密码会撤销该用户全部有效会话
- 密码使用 bcrypt，浏览器会话使用随机令牌，数据库只保存 HMAC-SHA256 哈希
- 控制台支持桌面侧边导航、移动端导航抽屉，以及系统、浅色和深色主题
- 用户 API Key 创建、一次性展示和撤销；管理员 Key 审计与撤销
- HTTPS 提供商配置、凭据加密、模型同步、模型目录和分维度单价维护
- 用户余额、余额流水、易支付在线充值、用量记录、原子余额调整和 `/v1` 模型网关
- 个人资料邀请链接、注册邀请码、充值确认后自动入余额的邀请返现，以及管理员返现比例设置
- 独立的 `/admin/payments` 易支付配置页；支持动态支付方式、充值金额、赠送档位、回调地址和全站充值记录

启动服务前应显式应用 `0001_initial_schema.sql` 初始化迁移。当前初始化不再内置固定模型目录；
只有上游同步返回或管理员手动创建的模型会进入选择器。上游价格不会写入 Novro，管理员必须在模型目录维护价格并启用模型。
没有启用的提供商或模型
路由时，`/v1/models`（以及兼容别名 `/v1/model`）会返回空列表，模型调用不会访问任何上游。列表只包含管理员从已同步目录中选中并启用的模型路由。

在线充值默认关闭。可在 `/admin/payments` 填写易支付配置并启用；API 地址可以是网关根地址或完整
`submit.php` 地址。首次部署也可以通过 `NOVRO_EPAY_API_URL`、`NOVRO_EPAY_MERCHANT_ID` 和
`NOVRO_EPAY_MERCHANT_KEY` 提供一次性引导默认值，服务会加密写入数据库，之后以管理页面为准。
`NOVRO_EPAY_CHANNELS` 仅用于首次引导默认渠道，页面保存后由数据库配置接管。
支付网关必须能访问 `${NOVRO_PUBLIC_URL}/api/payments/epay/notify`，该地址只接受通过
商户密钥验签且订单号、渠道、金额完全匹配的成功通知。浏览器支付完成后会先访问
`${NOVRO_PUBLIC_URL}/api/payments/epay/return`，后端使用相同校验幂等入账再返回余额页；它是
异步通知的补偿而不是替代。生产环境不能使用 `localhost` 作为公共地址。

为调用上游，管理员先在 `/admin/providers` 保存 HTTPS 提供商。系统会尝试同步上游模型；
管理员也可以从 `/admin/upstream-models` 模型目录手动多选，并在同一页面的“关联模型路由配置”
Tab 维护对外模型名。目录模型 ID 全局唯一，模型目录只维护一套人民币 / 百万 tokens 的各维度单价；同一模型关联多个提供商时自动形成故障切换池。用户创建自己的 Novro API Key
后，使用 `/v1` 前缀请求；同一对外模型名可关联多个启用渠道，网关按提供商权重从高到低选择渠道。
只有确认尚未取得连接的连接失败才会有限重试或切换；取得连接、收到响应或结果不明后不会自动重放。
调用前只预占一次余额，输入和输出默认最多分别按 16384 和 1024 Token 冻结；成功后只按上游
明确 usage 结算，多退少补。客户端应为可重试请求提供稳定的 `Idempotency-Key`。

## 8. 配置 Kimi（Moonshot）

Kimi 使用 OpenAI 兼容协议。管理员在 `/admin/providers` 新增提供商时可以使用以下配置，
API Key 只会加密保存在服务端，不会返回给浏览器：

```text
代码：moonshot
显示名称：Kimi / Moonshot
协议：OpenAI 兼容
基础地址：https://api.moonshot.cn/v1
API Key：从 Moonshot 控制台创建的密钥
```

新增成功后，从同步结果或模型目录多选需要开放的模型并建立关联路由。目录模型 ID 必须是 Moonshot
账户实际可用的值（例如 `kimi-k2.6` 或账户中显示的其他 Kimi 模型），自动关联时对外模型名使用同一个 ID。
各维度单价按人民币 / 百万 tokens 在全局目录维护一次；Novro 会按
上游返回的 `usage` 结算，没有 usage 的维度会按 0 结算并标记为不完整，不会把请求字节估算或
最大输出量当作实际消费。Kimi 的模型可用性、上下文
长度和价格会变化，以 Moonshot 控制台与官方文档为准。
