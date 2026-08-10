# Novro Docker 生产部署手册

本文是从一台全新的 Ubuntu/Debian 服务器部署 Novro 的完整操作手册。推荐使用本文的
Docker 单应用方案：一个 `novro` 容器内运行 Nginx、Go API/模型网关和 Next.js 控制台，
另一个只加入内部 Docker 网络的 MySQL 容器运行数据库。部署后公网只暴露 HTTPS，
不需要 Redis、消息队列或其他微服务。

源码进程部署、MySQL 备份恢复和灾备切换的补充规则见
[生产部署、备份与恢复](deployment.md)。本文中的命令以 Linux shell 为例；不要把生产
密码直接写进命令行参数、Git、镜像或聊天记录。

## 0. 先了解部署结果

部署完成后的请求路径是：

```text
浏览器 -- HTTPS :443 --> Nginx
                         |-- /api/*、/v1/*、/healthz、/readyz --> Go :8080
                         `-- 其他控制台页面 ------------------> Next.js :3000
Go :8080 -- 加密 Docker 网络 --> MySQL :3306
```

容器内的 Go 和 Next.js 只监听回环地址，MySQL 不发布宿主机端口。宿主机持久化目录默认
是 `/data/novro`：

| 路径 | 内容 | 处理要求 |
| --- | --- | --- |
| `/data/novro/mysql` | MySQL 数据文件 | 必须纳入主机磁盘备份，不要手工修改 |
| `/data/novro/tls` | `fullchain.pem`、`privkey.pem` | 私钥权限 `0600`，不要提交 Git |
| `/data/novro/.env.docker` | 数据库密码、会话密钥、应用配置 | 权限 `0600`，只给部署管理员读取 |
| `/data/novro/backups` | 可选的加密逻辑备份 | 存到独立故障域，不只放在本机 |

应用使用 MySQL 账号 `novro_app`，不是 `root`。MySQL 容器内部虽然需要 root 密码完成
初始化，但应用不会使用它连接业务数据。

## 1. 上线前清单

部署前确认下面每一项，缺任何一项都先停在清单阶段：

- 一台 Ubuntu 22.04/24.04 或 Debian 12/13 的 x86-64 服务器，建议至少 2 vCPU、4 GB RAM、
  40 GB SSD；模型代理的并发量较大时按实际流量扩容。
- 一个已经解析到服务器公网 IPv4 的域名，例如 `novro.example.com`。若使用 IPv6，必须
  同时确认防火墙和 Docker 的 IPv6 策略；首次部署建议只配 IPv4。
- DNS 的 A 记录已经生效：在任意机器执行
  `dig +short novro.example.com`，结果应包含服务器地址。
- 服务器可出站访问 Docker Hub、上游模型、SMTP、OIDC 和支付网关；只开放必要的入站端口。
- 可信 HTTPS 证书和匹配私钥。可以先用自签名证书验证安装，再换成受信任证书；正式用户流量
  不应长期使用自签名证书。
- 一个用于首次登录的随机管理员密码。它必须至少 8 位并同时包含英文字符和数字；部署后还要
  在控制台中更换为长期密码。
- 一个独立的 MySQL 备份位置和恢复演练安排。备份只保留在服务器本机不算完成备份。

当前仓库要求生产配置满足：数据库连接启用 TLS、会话 Cookie 使用 `Secure`、公共地址和
允许来源均为 HTTPS Origin，Go 服务监听回环地址。配置不符合这些条件时，生产模式会直接拒绝启动。

## 2. 服务器和防火墙准备

以下示例假定服务器是 Ubuntu/Debian，并使用非 root 的 `deploy` 用户维护代码。把
`deploy` 替换成你的运维用户名；不要把应用运行在 root shell 中。

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl git openssl ufw
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG sudo deploy
sudo install -d -o deploy -g deploy -m 0750 /opt/novro
sudo install -d -o deploy -g deploy -m 0700 /data/novro
```

如果服务器使用 UFW，先允许 SSH，再开放 HTTP/HTTPS。确认 SSH 会话有第二条可用连接后再
启用防火墙，避免把自己锁在服务器外：

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw enable
sudo ufw status verbose
```

不要开放 `3306`。本方案的 MySQL 只在 Docker 内部网络可见；如果使用外部 MySQL，则只允许
应用服务器固定 IP 访问，并保持 `NOVRO_DATABASE_TLS=true`。不要把数据库端口开放给整个公网。

## 3. 获取发布代码

生产环境应部署已经审查、测试并标记的 commit/tag，不要直接在服务器上开发。下面用 SSH
远程仓库举例；私有仓库请先为 `deploy` 用户配置只读 Deploy Key 或受限机器账号。

```bash
sudo -iu deploy
cd /opt/novro
git clone git@github.com:novro-gateway/novro.git .
git fetch --tags --prune origin
git status --short
```

工作区必须干净。部署某个已审查的版本，例如：

```bash
export NOVRO_RELEASE='v1.0.0'       # 或填已审查的完整 commit SHA
git checkout --detach "$NOVRO_RELEASE"
git rev-parse HEAD
```

把输出的 commit SHA 记录到发布单。升级时沿用同一个目录，不要重新生成
`/data/novro/.env.docker`；其中包含数据库和加密密钥，丢失会导致无法解密已保存的提供商、
SMTP 或支付凭据。

## 4. 申请或准备 HTTPS 证书

### 4.1 使用已有证书（正式推荐）

证书文件应是 PEM 格式，私钥必须与证书匹配。部署前在服务器上确认文件可读：

```bash
sudo openssl x509 -in /etc/letsencrypt/live/novro.example.com/fullchain.pem -noout -subject -dates
sudo openssl x509 -noout -modulus -in /etc/letsencrypt/live/novro.example.com/fullchain.pem | openssl sha256
sudo openssl rsa  -noout -modulus -in /etc/letsencrypt/live/novro.example.com/privkey.pem   | openssl sha256
```

两条 SHA-256 输出必须一致。部署脚本会把它们复制为
`/data/novro/tls/fullchain.pem` 和 `/data/novro/tls/privkey.pem`，不会把原路径挂载进容器。

### 4.2 用 Certbot 临时申请证书

如果 80/443 尚未被其他服务占用，可以在首次部署前申请：

```bash
sudo apt-get install -y certbot
sudo certbot certonly --standalone \
  --agree-tos --no-eff-email \
  -m ops@example.com \
  -d novro.example.com
```

申请成功后使用 `4.1` 的路径执行部署。证书续期后，必须重新复制证书并重载容器内 Nginx；
仅更新 `/etc/letsencrypt` 不会自动更新已经复制到 `/data/novro/tls` 的文件：

```bash
sudo install -m 0644 /etc/letsencrypt/live/novro.example.com/fullchain.pem /data/novro/tls/fullchain.pem
sudo install -m 0600 /etc/letsencrypt/live/novro.example.com/privkey.pem /data/novro/tls/privkey.pem
cd /opt/novro
sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker exec -T novro nginx -t
sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker exec -T novro nginx -s reload
```

可以把上述复制和 `nginx -t`/reload 放到 Certbot 的 deploy hook 中。hook 失败时不要删除
旧证书，先检查 Nginx 配置和证书权限。

### 4.3 仅用于安装验证的自签名证书

没有域名或证书时，脚本会自动生成 `/data/novro/tls/` 下的自签名证书：

```bash
sudo bash scripts/deploy-docker.sh --domain localhost
```

浏览器会显示证书不受信任，这是预期结果。正式切换前必须使用可信证书重新执行带
`--tls-cert` 和 `--tls-key` 的部署命令，并从客户端验证证书链和域名。

## 5. 首次一键部署（推荐路径）

### 5.1 安全输入引导密码

进入 root 的交互 shell 后再输入密码，避免密码出现在普通用户的 shell 历史或命令参数中：

```bash
sudo -i
cd /opt/novro
read -r -s -p 'Novro 初始管理员密码: ' NOVRO_BOOTSTRAP_PASSWORD
printf '\n'
export NOVRO_BOOTSTRAP_PASSWORD
```

脚本会检查密码长度、字符类型，并在管理员创建和第一次容器重建后从
`/data/novro/.env.docker` 及运行中的容器环境中清空该值。脚本不会打印密码。

### 5.2 使用可信证书启动

```bash
bash scripts/deploy-docker.sh \
  --domain novro.example.com \
  --tls-cert /etc/letsencrypt/live/novro.example.com/fullchain.pem \
  --tls-key /etc/letsencrypt/live/novro.example.com/privkey.pem
```

脚本按以下顺序执行：

1. 安装或检查 Docker Engine、Buildx 和 Compose 插件。
2. 在 `/data/novro/.env.docker` 生成随机的 MySQL 应用密码、MySQL root 密码、会话密钥和
   提供商加密密钥；已有环境文件不会被覆盖。
3. 创建或复制 TLS 证书，并检查 Nginx 配置。
4. 构建 Go 二进制和 Next.js standalone 镜像。
5. 启动 MySQL，等待健康检查通过。
6. 执行 `check-db`，再执行显式版本化 `migrate`。服务正常启动不会自动建表。
7. 如果数据库尚未初始化，执行幂等的 `bootstrap-admin` 创建第一个系统管理员。
8. 启动 supervisor 管理的 Go、Next.js、Nginx，并通过 HTTPS `/readyz` 和 `/login` 检查。
9. 清空引导密码并重建应用容器，再次执行就绪检查。

部署结束后执行：

```bash
unset NOVRO_BOOTSTRAP_PASSWORD
grep '^NOVRO_BOOTSTRAP_PASSWORD=' /data/novro/.env.docker
docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker ps
exit
```

`grep` 的结果必须是空值，例如 `NOVRO_BOOTSTRAP_PASSWORD=`。如果仍有值，先不要把终端
输出发给他人，手工清空该行并执行：

```bash
sudo chmod 600 /data/novro/.env.docker
sudo sed -i 's/^NOVRO_BOOTSTRAP_PASSWORD=.*/NOVRO_BOOTSTRAP_PASSWORD=/' /data/novro/.env.docker
sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker up -d --force-recreate --no-deps novro
```

### 5.3 已有宿主机网关时的端口绑定

如果宿主机 Nginx、Caddy 或其他程序已经占用 80/443，应用容器只绑定回环地址和未占用端口：

```bash
export NOVRO_BIND_ADDRESS=127.0.0.1
export NOVRO_HTTP_PORT=18081
export NOVRO_HTTPS_PORT=18443
bash scripts/deploy-docker.sh \
  --domain novro.example.com \
  --tls-cert /etc/letsencrypt/live/novro.example.com/fullchain.pem \
  --tls-key /etc/letsencrypt/live/novro.example.com/privkey.pem
unset NOVRO_BIND_ADDRESS NOVRO_HTTP_PORT NOVRO_HTTPS_PORT
```

宿主机网关必须把 `/api/*`、`/v1/*`、`/healthz`、`/readyz` 以及控制台页面转发到 HTTPS
端口（此例为 `18443`），保留 `Host`、`Origin`、`X-Forwarded-Proto=https`，并关闭 `/v1/*`
的响应缓冲。不要把内部 Go `8080` 或 Next `3000` 直接发布到公网。

## 6. 首次登录和应用配置

1. 打开 `https://novro.example.com/login`，使用部署时填写的管理员用户名（默认 `novro`）
   和引导密码登录。
2. 进入管理员用户页面，为日常操作创建第二个管理员账号并设置独立密码；保留系统管理员
   作为受保护的恢复账号。
3. 立即在管理员用户页面重置系统管理员密码。重置会撤销该账号的所有旧会话。
4. 确认引导密码已经从环境文件删除，并从密码管理器中删除临时记录或按组织策略归档。
5. 如果开放普通用户注册，在 `/admin/email` 配置 SMTP 并发送测试邮件。生产环境没有完整
   SMTP 配置时，验证码不会写入日志，注册会返回不可用；不要把 SMTP 密码放到
   `NEXT_PUBLIC_*` 变量。
6. 如需企业登录，在身份平台登记
   `https://novro.example.com/api/auth/oidc/callback`，再在部署秘密中同时设置
   `NOVRO_OIDC_ISSUER`、`NOVRO_OIDC_CLIENT_ID`、`NOVRO_OIDC_CLIENT_SECRET` 并重建容器。
7. 如需在线充值，在 `/admin/payments` 配置 HTTPS 易支付地址、商户号、密钥、回调渠道和
   金额。平台必须能访问：
   `https://novro.example.com/api/payments/epay/notify`；同步返回地址是
   `https://novro.example.com/api/payments/epay/return`。
8. 在 `/admin/providers` 只添加 HTTPS 上游，保存后同步模型；在模型目录维护单价并启用
   对外路由。提供商 API Key 会在服务端加密保存，不会返回浏览器。
9. 创建一个普通用户 API Key。Key 只在创建成功时完整展示一次；将它放进受限的客户端秘密
   存储，不要写入前端代码、访问日志或工单。

## 7. 上线验收

### 7.1 容器和本机检查

```bash
cd /opt/novro
sudo docker compose --env-file /data/novro/.env.docker ps
sudo docker compose --env-file /data/novro/.env.docker logs --tail=100 novro
sudo docker compose --env-file /data/novro/.env.docker exec -T novro curl --fail http://127.0.0.1:8080/healthz
sudo docker compose --env-file /data/novro/.env.docker exec -T novro curl --fail http://127.0.0.1:8080/readyz
sudo docker compose --env-file /data/novro/.env.docker exec -T novro nginx -t
```

`healthz` 只证明 Go 进程存活；`readyz` 会在两秒内检查数据库。`docker compose ps` 中
`mysql` 应为 `healthy`，`novro` 应为 `Up`。日志中不应出现密码、Cookie、Authorization
头或上游 API Key。

### 7.2 从公网检查

先用域名验证证书、代理和控制台：

```bash
curl --fail --silent --show-error https://novro.example.com/healthz
curl --fail --silent --show-error https://novro.example.com/readyz
curl --fail --silent --show-error https://novro.example.com/login >/dev/null
curl --fail --silent --show-error https://novro.example.com/docs >/dev/null
```

然后在浏览器完成登录、控制台导航、管理员页面加载和退出。最后使用刚创建的 API Key 做
一个不产生费用的模型列表请求：

```bash
export NOVRO_API_KEY='在受控终端临时读取的 API Key'
curl --fail --silent --show-error \
  -H "Authorization: Bearer $NOVRO_API_KEY" \
  https://novro.example.com/v1/models
unset NOVRO_API_KEY
```

如果已配置提供商和模型路由，再做一个极小的受控模型请求，确认响应、usage、钱包预占和
结算流水都存在。没有启用模型时 `/v1/models` 返回空列表是正常的，不代表网关故障。

上线验收记录至少包括：发布 commit SHA、迁移版本、`readyz` 时间、证书到期时间、备份文件
和 SHA-256、管理员登录结果、模型列表结果以及操作者。

## 8. 手工启动和日常命令

脚本无法使用时，复制模板并只在服务器上填写真实值：

```bash
sudo install -d -m 0700 /data/novro/tls /data/novro/mysql
sudo cp /opt/novro/deploy/docker.env.example /data/novro/.env.docker
sudo chmod 600 /data/novro/.env.docker
# 编辑 /data/novro/.env.docker；至少填写 URL、两个 32 字节密钥、数据库密码和引导密码
sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker config --quiet
sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker up -d --build
```

本节及后续升级命令使用 `sudo docker`，因此不依赖当前 shell 是否已加入 Docker 组。若希望
省略 `sudo`，Docker 安装完成后可把可信运维用户加入 Docker 组，然后退出并重新登录：

```bash
sudo usermod -aG docker deploy
```

入口脚本顺序固定为：检查证书、`nginx -t`、等待 MySQL、`check-db`、`migrate`、可选的
`bootstrap-admin`，最后启动 supervisor。正常升级不要执行 `down -v`；它容易被误用为
删除数据库的操作。常用运维命令：

```bash
COMPOSE=(sudo docker compose --project-directory /opt/novro --env-file /data/novro/.env.docker)
"${COMPOSE[@]}" ps
"${COMPOSE[@]}" logs --tail=200 novro
"${COMPOSE[@]}" logs --since=1h mysql
"${COMPOSE[@]}" restart novro
"${COMPOSE[@]}" stop
"${COMPOSE[@]}" start
```

停止/启动不会删除 bind mount 中的数据库文件。仅在明确要销毁整套环境并且已经完成独立备份
后，才考虑删除容器和数据目录；生产环境不要用 `docker compose down -v` 代替停机。

## 9. 备份和恢复

每次升级前先做逻辑备份；备份文件写完并校验后，才允许停止写流量。生产备份应使用独立的
最小权限账号或数据库平台的加密备份。仓库自带的 PowerShell 脚本适用于 Windows 运维机：

```powershell
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
./scripts/mysql-backup.ps1 -OutputPath "./backups/novro-$stamp.sql"
```

Docker 主机也可以在 MySQL 容器内执行 `mysqldump`，密码只从容器环境读取，不作为参数传给
`mysqldump`。下面命令会保留一个 `.partial`，只有成功后才改名：

```bash
cd /opt/novro
mkdir -p /data/novro/backups
STAMP="$(date -u +%Y%m%d-%H%M%S)"
OUT="/data/novro/backups/novro-${STAMP}.sql"
TMP="${OUT}.partial"
sudo docker compose --env-file /data/novro/.env.docker exec -T mysql sh -ceu '
  option_file=/tmp/novro-backup.cnf
  trap "rm -f \"$option_file\"" EXIT
  umask 077
  printf "[client]\\nuser=root\\npassword=%s\\n" "$MYSQL_ROOT_PASSWORD" > "$option_file"
  mysqldump --defaults-extra-file="$option_file" --single-transaction --routines --events --triggers --hex-blob --set-gtid-purged=OFF "$MYSQL_DATABASE"
' > "$TMP"
test -s "$TMP"
mv "$TMP" "$OUT"
sha256sum "$OUT" | tee "$OUT.sha256"
chmod 600 "$OUT" "$OUT.sha256"
```

把 `.sql` 和 `.sha256` 一起复制到加密且独立于数据库主机的存储，并按保留策略清理旧备份。
至少每月在以 `novro_restore_YYYYMMDD_HHMMSS` 开头的新数据库中做一次隔离恢复演练。恢复时：

1. 校验备份 SHA-256，不要跳过校验文件。
2. 恢复到新库，不要覆盖当前生产库。
3. 使用只读检查账号抽查管理员、钱包、流水、Key 前缀、提供商和模型路由数量；不要导出
   密码哈希、Key 哈希或加密凭据。
4. 对恢复库执行 `check-db` 和 `migrate`，确认迁移 checksum 是当前发布包的连续前缀。
5. 停止写流量后，将 `NOVRO_DATABASE_NAME` 改为恢复库，重启应用并确认 `/readyz`，再逐步
   恢复读流量和写流量。
6. 原生产库保持只读并归档，确认新库稳定后再按审批流程处理旧库。

更完整的 MySQL 权限、行数比较和恢复限制见 [生产部署、备份与恢复](deployment.md) 第 7--9 节。

## 10. 升级流程

Novro 的数据库迁移是显式、按文件名顺序、前向执行的；服务启动不会自动修改结构。每次升级
都应按以下顺序操作：

1. 阅读新版本变更说明，确认是否新增迁移、配置变量或上游兼容性要求。
2. 在 CI 或发布工作区依次执行完整检查：

   ```bash
   go test ./cmd/... ./internal/... ./ent/...
   go vet ./...
   pnpm --dir apps/web lint
   pnpm --dir apps/web typecheck
   pnpm --dir apps/web test
   pnpm --dir apps/web build
   git diff --check
   ```

   Next.js 的 `build` 和 `typecheck` 要顺序执行，避免并发产生过期的路由类型文件。
3. 在生产机记录当前 SHA、执行备份并确认备份文件可读和校验值已上传。
4. 在维护窗口停止或排空写流量，保持旧容器直到备份和发布文件都确认无误。
5. 获取目标 release 并检查工作区干净：

   ```bash
   cd /opt/novro
   git fetch --tags --prune origin
   git checkout --detach "$NOVRO_RELEASE"
   git status --short
   git rev-parse HEAD
   ```

6. 检查 Compose 渲染结果，再构建并启动：

   ```bash
   sudo docker compose --env-file /data/novro/.env.docker config --quiet
   sudo docker compose --env-file /data/novro/.env.docker up -d --build
   ```

   入口脚本会在新 API 进程启动前执行待处理迁移。如果迁移失败，先保留现场并查看日志，
   不要手工删 `novro_schema_migrations` 或直接修改 SQL。
7. 按第 7 节完成容器、本机和公网验收；确认数据库迁移版本、登录、模型列表以及受控网关
   请求后，再恢复全部流量。
8. 记录新 SHA、迁移版本、开始/结束时间、备份文件和验收结果。

配置或代码回滚（数据库没有执行不兼容迁移时）：

```bash
git checkout --detach "$PREVIOUS_RELEASE"
sudo docker compose --env-file /data/novro/.env.docker up -d --build
```

一旦新版本迁移已经改变生产结构，不要把代码直接回退到不认识该结构的旧版本。应停止写流量，
按第 9 节恢复到新的恢复库，并让一个兼容当前结构的版本完成迁移和验收；迁移 SQL 不提供自动
回滚。`NOVRO_SESSION_SECRET` 变更会使所有现有会话失效；`NOVRO_PROVIDER_ENCRYPTION_SECRET`
丢失或随意更换会使已保存的提供商、SMTP 和支付密钥无法解密，轮换必须先设计数据重加密流程。

## 11. 证书、密码和密钥轮换

- **TLS 证书**：替换 `/data/novro/tls` 中两个文件，先执行 `nginx -t`，再执行 `nginx -s reload`。
- **数据库密码**：先在 MySQL 创建/修改 `novro_app` 密码，再更新 `.env.docker`，执行
  `docker compose up -d --force-recreate --no-deps novro`，最后检查 `/readyz`。
- **会话密钥**：在维护窗口更新 `NOVRO_SESSION_SECRET` 并重建应用；所有登录会话会失效，
  用户需要重新登录。
- **提供商加密密钥**：不能直接替换。先实现并验证批量解密-重加密、备份和回滚方案，再按维护
  窗口轮换；否则历史提供商、SMTP 和支付凭据会不可用。
- **上游 API Key、SMTP、OIDC、支付密钥**：优先在管理员页面替换；环境变量仅作为首次引导
  兜底。替换后检查一次真实功能并从旧密钥提供方撤销旧值。

## 12. 故障排查

### 容器反复退出

```bash
docker compose --env-file /data/novro/.env.docker ps
docker compose --env-file /data/novro/.env.docker logs --tail=200 novro
docker compose --env-file /data/novro/.env.docker logs --tail=200 mysql
```

优先检查：TLS 两个文件是否存在且可读、`docker compose config --quiet` 是否通过、MySQL 是否
为 `healthy`、生产 URL 是否为 HTTPS、两个 32 字节密钥是否存在。不要把完整日志原样发到公共
工单；先删除可能包含凭据的行。

### `readyz` 失败

确认 MySQL 容器健康、应用账号密码匹配、数据库名正确、Docker 网络存在，并从应用容器内执行：

```bash
docker compose --env-file /data/novro/.env.docker exec -T novro /usr/local/bin/novro check-db
```

云 MySQL 如果报 `x509: certificate signed by unknown authority`，应安装正确 CA 或使用受控
隧道；不要关闭 TLS，也不要退回明文连接。生产配置要求 `NOVRO_DATABASE_TLS=true`。

### 登录页能打开但 API 操作失败

这通常是旧 Go 容器、旧数据库结构或代理路由指向错误实例。按顺序确认 `ps`、`readyz`、迁移
日志和当前 commit SHA，重建匹配版本的 `novro` 容器后再重复浏览器流程。不要只凭前端页面能
渲染就判断部署成功。

### 模型请求失败或余额不变

在控制台确认提供商为启用状态、模型已同步/选中、对外路由启用且单价已维护。检查请求返回的
`request_id`，再对照 `api_usages` 和钱包流水。出现 `finalize gateway usage` 或
`refund gateway reservation` 重试耗尽时，先保留日志、提供商响应和流水记录，再由管理员做
人工对账；不要直接改数据库余额。

### 忘记系统管理员密码

在维护窗口临时注入密码，执行一次性重置，完成后立即清除变量：

```bash
sudo -i
cd /opt/novro
read -r -s -p '新的系统管理员密码: ' NOVRO_ADMIN_PASSWORD
printf '\n'
export NOVRO_ADMIN_PASSWORD
docker compose --env-file /data/novro/.env.docker exec -T -e NOVRO_ADMIN_PASSWORD novro /usr/local/bin/novro reset-admin
unset NOVRO_ADMIN_PASSWORD
exit
```

如果应用容器内没有该环境变量，使用临时 Compose 覆盖或在受控 root shell 中执行对应命令；
不要把密码写入 `.env.docker`、Compose 文件或镜像。重置后检查登录并撤销不需要的会话。

## 13. 明确禁止的操作

- 不要执行 `docker compose down -v`、`rm -rf /data/novro` 或直接删除 MySQL 数据目录。
- 不要在生产服务启动时依赖 Ent 自动建表；必须先显式运行 `migrate`。
- 不要修改已应用的迁移 SQL，不要删除 `novro_schema_migrations`。
- 不要让应用使用 MySQL `root`，不要把 `3306` 暴露给公网。
- 不要在 Nginx 访问日志记录 `Authorization`、Cookie、请求正文或上游 API Key。
- 不要把真实 `.env.docker`、证书私钥、备份、密码、会话密钥或提供商凭据提交到 Git。
- 不要在没有已验证备份和恢复方案的情况下做生产升级或数据库切换。

## 14. 部署完成记录模板

每次首次部署、升级、证书更新和恢复切换都保存一条记录：

```text
时间（UTC）：
环境/域名：
服务器：
Novro commit/tag：
数据库名和迁移版本：
备份文件及 SHA-256：
证书到期时间：
healthz/readyz：
登录和控制台验收：
/v1/models 验收：
受控模型请求及 request_id（如有）：
异常、人工对账或已知限制：
操作者和审批人：
```
