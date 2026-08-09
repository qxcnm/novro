# Docker 单应用部署

这套方案把 Novro 的三个应用进程放进一个应用容器：Nginx 负责 HTTPS 和反向代理，Go
负责 API/模型网关，Next.js 负责控制台。MySQL 只作为内部数据库容器运行，不发布主机
端口。对外只有应用容器的 `80` 和 `443`，因此部署层只有一个 Novro 应用服务。默认将
数据库和 TLS 数据持久化到宿主机 `/data/novro`，可通过 `NOVRO_DATA_DIR` 覆盖。

## 1. 一键部署

支持 Ubuntu 和 Debian 主机。脚本会安装或补齐 Docker Engine、Docker Compose 插件、
OpenSSL 和必要的系统工具，然后生成随机数据库/会话/提供商密钥，创建 TLS 证书，构建
镜像，启动 MySQL，等待数据库就绪，执行版本化迁移，创建初始管理员，最后检查 HTTPS
就绪状态：

```bash
sudo bash scripts/deploy-docker.sh --domain novro.example.com
```

没有可用域名时可先用本机地址验证：

```bash
sudo bash scripts/deploy-docker.sh --domain localhost
```

首次没有提供证书时脚本会在 `/data/novro/tls/` 生成自签名证书，浏览器会提示证书不受信任。
正式环境请把受信任的证书和匹配私钥交给脚本：

```bash
sudo bash scripts/deploy-docker.sh \
  --domain novro.example.com \
  --tls-cert /etc/letsencrypt/live/novro.example.com/fullchain.pem \
  --tls-key /etc/letsencrypt/live/novro.example.com/privkey.pem
```

如果宿主机已经有 Nginx 或其他网关占用 `80/443`，可让应用只绑定回环地址和未占用端口，
再由宿主机按域名反向代理：

```bash
export NOVRO_BIND_ADDRESS=127.0.0.1
export NOVRO_HTTP_PORT=18081
export NOVRO_HTTPS_PORT=18443
sudo -E bash scripts/deploy-docker.sh --domain novro.example.com
```

`NOVRO_BIND_ADDRESS` 默认是 `0.0.0.0`；生产环境接入已有网关时建议使用回环绑定，
避免绕过宿主机的 TLS、访问控制和日志策略。

脚本不会把初始化密码打印到终端。首次启动的管理员账号默认是 `novro`，密码必须通过未入库的
`NOVRO_BOOTSTRAP_PASSWORD` 环境变量提供，并满足至少 8 位、包含英文和数字的要求。初始化完成后，
脚本会从 `/data/novro/.env.docker` 和运行中的应用容器中移除引导密码。若要重新初始化一个全新的
数据库，必须在部署前重新设置该环境变量。

## 2. 文件与职责

| 文件 | 作用 |
| --- | --- |
| `Dockerfile` | 多阶段构建 Go 二进制和 Next.js standalone 产物，运行时安装 Nginx 与 supervisor |
| `compose.yaml` | 单个 `novro` 应用服务加内部 `mysql` 服务，MySQL 不发布端口 |
| `deploy/nginx.conf` | HTTPS、`/api/*`、`/v1/*`、健康检查和控制台路由 |
| `deploy/supervisord.conf` | 在一个应用容器内监督 Go、Next.js 和 Nginx |
| `deploy/docker-entrypoint.sh` | 检查证书、等待 MySQL、执行迁移和一次性管理员引导 |
| `scripts/deploy-docker.sh` | 主机软件安装、密钥/证书生成、构建、启动和就绪检查 |
| `deploy/docker.env.example` | 手工部署时的变量模板，不含真实密钥 |

持久化目录约定：`/data/novro/mysql` 保存 MySQL 数据，`/data/novro/tls` 保存证书和私钥，
`/data/novro/.env.docker` 保存部署运行配置。上述目录不应加入 Git 或公开分享。

正常运行时的请求路径如下：

```text
Browser --HTTPS--> Nginx :443
                    |-- /api/*, /v1/*, /healthz, /readyz --> Go :8080
                    `-- other console routes ----------------> Next :3000
Go :8080 --TLS--> MySQL :3306 (Docker network only)
```

Nginx 对所有路由不限制请求体大小。模型网关 `/v1/*` 还会关闭请求/响应缓冲、响应压缩、
代理缓存、错误拦截和响应限速，Nginx 的客户端请求体、上游读写和客户端发送空闲超时统一
放大到 24 天，保证长上下文、SSE 和较慢的上游响应不会被代理提前截断。Nginx 的这些
读写定时器不能用 `0` 表示关闭，因此使用低于内部毫秒计时器边界的 24 天作为实际无限值。

Go 服务不设置 HTTP 请求读写超时；模型上游客户端将连接、TLS 握手和等待响应头分别限制为
15 秒、30 秒和 180 秒，收到响应头后的流式模型调用不设整体超时，避免上游黑洞让下游无限等待。
网关请求体、非流式响应、SSE 单行、后台 JSON 和支付回调表单均无固定字节上限；
输出 token 参数不再使用固定上限，只接受程序整数范围内的正整数。鉴权、Origin 校验、JSON
结构校验、支付签名、计费预占、TLS 和上游 SSRF 防护仍然生效。

应用配置保持生产安全约束：Go 只监听容器内回环地址，数据库连接强制 TLS（证书链使用
受控环境下的 `skip-verify`），Cookie 使用 `Secure`，MySQL 运行账号是 `novro_app`，
不是 `root`。模型上游、SMTP、OIDC 和支付网关仍由应用通过出站网络访问。

## 3. 手工启动

脚本不能运行时，可以复制模板并手动填写真实值：

```bash
cp deploy/docker.env.example /data/novro/.env.docker
chmod 600 /data/novro/.env.docker
mkdir -p /data/novro/tls /data/novro/mysql
# 将 fullchain.pem 和 privkey.pem 放入 /data/novro/tls/
docker compose --env-file /data/novro/.env.docker config --quiet
docker compose --env-file /data/novro/.env.docker up -d --build
```

首次手工启动需要在 `.env.docker` 中暂时保留 `NOVRO_BOOTSTRAP_PASSWORD`。容器入口会按
顺序执行：数据库连接检查、迁移、`bootstrap-admin`，然后交给 supervisor 启动三个应用
进程。`bootstrap-admin` 对已经初始化的数据库幂等返回，不会重复创建管理员。

常用检查：

```bash
docker compose --env-file /data/novro/.env.docker ps
docker compose --env-file /data/novro/.env.docker logs --tail=100 novro
docker compose --env-file /data/novro/.env.docker exec novro curl --fail http://127.0.0.1:8080/readyz
```

不要执行 `docker compose down -v`，除非已经确认要删除 MySQL 数据卷；普通升级只需：

```bash
docker compose --env-file /data/novro/.env.docker up -d --build
```

## 4. 证书更新

证书是只读挂载到应用容器的。替换 `/data/novro/tls/fullchain.pem` 和
`/data/novro/tls/privkey.pem` 后重载 Nginx：

```bash
docker compose --env-file /data/novro/.env.docker exec novro nginx -t
docker compose --env-file /data/novro/.env.docker exec novro nginx -s reload
```

证书申请和续期由宿主机或证书平台负责；续期后必须执行上述重载。不要把真实私钥加入
Git，`.gitignore` 已忽略 `.pem` 和 `.key` 文件。

## 5. 停止与备份

```bash
docker compose --env-file /data/novro/.env.docker stop
docker compose --env-file /data/novro/.env.docker start
```

停止不会删除数据。备份前先执行 `docker compose ... ps` 确认 MySQL 正常，再使用现有
备份脚本或数据库平台的加密备份；恢复和迁移规则仍以[生产部署、备份与恢复](deployment.md)
为准。
