# 开发启动说明

本文只覆盖本地开发。生产变量、构建、反向代理、上线、回滚、备份和恢复流程见
[部署与恢复](deployment.md)。

## 1. 前置条件

- Go `1.26.5`
- Node.js `24` 与 pnpm `10.30.3`
- MySQL `8.4`，运行账号必须是最小权限的专用账号
- 可访问的云端 MySQL 实例

复制 `.env.example` 中的变量名到本机安全环境。不要把真实 `.env`、数据库密码、
会话密钥或 CA 私钥提交到 Git。前端仅使用服务端变量 `NOVRO_SERVER_URL`，默认
连接 `http://127.0.0.1:8080`，该变量不得使用 `NEXT_PUBLIC_` 前缀。

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

数据库名以部署环境为准，当前开发实例使用 `novro-db`。应用运行时不要使用
`root`。当前实例直接设置：

```env
NOVRO_DATABASE_TLS=true
```

当前 MySQL 客户端保持 TLS 加密但不验证证书链，与现有 DBeaver 连接方式一致；
`NOVRO_DATABASE_TLS=false` 只允许用于明确受控的本地开发环境。
首次使用管理员账号创建数据库，然后切换到应用账号验证连接并显式执行迁移：

```powershell
go run ./cmd/novro init-db
go run ./cmd/novro check-db
go run ./cmd/novro migrate
```

`init-db` 只创建配置中的数据库，不创建表；正常服务启动不会调用该命令。

迁移位于 `ent/migrate/migrations`，需要与 Ent Schema 一起审查。服务正常启动时
不会自动改变数据库结构。

## 4. 初始化管理员

迁移完成后，推荐生成一次性初始化令牌并启动服务：

```powershell
$env:NOVRO_SETUP_TOKEN='<GENERATED_RANDOM_VALUE>'
go run ./cmd/novro
```

访问 `http://localhost:3000/setup`，输入该令牌并创建管理员。成功后从部署环境删除
`NOVRO_SETUP_TOKEN`。数据库会原子地关闭初始化入口，不能通过重复请求创建第二个
初始管理员，也没有内置默认密码。

无浏览器环境仍可通过当前进程环境变量创建第一个管理员：

```powershell
$env:NOVRO_BOOTSTRAP_USERNAME='admin'
$env:NOVRO_BOOTSTRAP_DISPLAY_NAME='Administrator'
$env:NOVRO_BOOTSTRAP_PASSWORD='<LOCAL_SECRET>'
go run ./cmd/novro bootstrap-admin
Remove-Item Env:NOVRO_BOOTSTRAP_PASSWORD
```

密码必须为 12 到 72 字节。命令不接受明文密码参数，也不会打印密码。

## 5. 注册与 OIDC

`NOVRO_REGISTRATION_ENABLED=true` 允许公开注册普通成员，不能注册管理员。首次
管理员初始化完成前，注册和 OIDC 自动注册都会被拒绝。

配置第三方 OIDC 时，在身份平台登记回调地址：

```text
http://localhost:3000/api/auth/oidc/callback
```

生产环境应改为 `NOVRO_PUBLIC_URL` 对应的 HTTPS 域名。OIDC 配置示例：

```env
NOVRO_PUBLIC_URL=https://novro.example.com
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

- 用户登录、退出和当前用户查询
- 管理员用户列表、搜索、筛选、分页、创建、资料与角色编辑、启用、停用和密码重置
- 最后一个启用管理员不能被停用或降级
- 重置密码会撤销该用户全部有效会话
- 密码使用 bcrypt，浏览器会话使用随机令牌，数据库只保存 HMAC-SHA256 哈希
- 控制台支持桌面侧边导航、移动端导航抽屉，以及系统、浅色和深色主题
- 用户 API Key 创建、一次性展示和撤销；管理员 Key 审计与撤销
- HTTPS 提供商配置、凭据加密、模型路由和输入/输出单价维护
- 用户余额、余额流水、用量记录、原子余额调整和 `/v1` 模型网关

启动服务前必须先执行包含 `0003` 到 `0006` 的显式迁移。没有启用的提供商或模型
路由时，`/v1/models` 会返回空列表，模型调用不会访问任何上游。

为调用上游，管理员先在 `/admin/providers` 保存 HTTPS 提供商，再在 `/admin/models`
创建对外模型名、上游模型名和人民币 / 百万 tokens 单价。用户创建自己的 Novro API Key
后，使用 `/v1` 前缀请求；网关会在调用前预占余额，成功后按上游 usage 结算。
