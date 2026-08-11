# Novro 三种部署方式：GHCR、离线包、服务器本地构建

本文覆盖 Novro 从镜像发布或打包，到服务器启动、Nginx 配置、域名绑定和验证的完整流程。
文档中的 $ip$、$域名$、$ghcr用户$、$仓库名$ 和 $版本$ 都是占位符，执行前替换成实际值。
本文不包含真实 IP、域名、密码、Token 或密钥。

## 一、方式选择

| 方式 | 镜像来源 | 服务器联网要求 | 适用场景 |
| --- | --- | --- | --- |
| GHCR 镜像部署 | ghcr.io/$ghcr用户$/$仓库名$:$版本$ | 需要访问 GHCR 和 Docker Registry | 常规持续部署，推荐 |
| 离线包部署 | 本地生成的 novro-images.tar | 不需要访问 Registry | 网络受限或隔离环境 |
| 服务器本地构建 | 服务器上的源码和 Dockerfile | 需要访问构建依赖源 | 临时测试或没有镜像仓库 |

三种方式共用一个 Compose 应用结构：

- novro 容器内部运行 Nginx、Go API 和 Next.js。
- mysql:8.4 容器只加入内部 Docker 网络。
- /data/novro/mysql 保存数据库数据。
- /data/novro/.env.docker 保存数据库密码、会话密钥和应用配置。
- /data/novro/nginx.conf 是当前生效的 Nginx 配置。
- Compose 不应把 MySQL 的 3306 映射到公网。

不要执行 docker compose down -v，也不要删除 /data/novro/mysql。

## 二、通用前置条件

### 本地电脑

~~~powershell
cd C:\path\to\novro
docker info --format 'OS={{.OSType}} Architecture={{.Architecture}}'
docker buildx version
~~~

Docker 必须使用 Linux containers。x86_64 服务器使用 linux/amd64；ARM64 服务器使用
linux/arm64。不要把 amd64 镜像导入 ARM64 服务器运行。

### 服务器

~~~bash
docker --version
docker compose version
curl --version
openssl version
uname -m
~~~

安全组或防火墙至少放行：

~~~text
22/tcp   SSH
80/tcp   HTTP 或 ACME HTTP-01
443/tcp  HTTPS
~~~

不要放行 3306/tcp。服务器预先存在 mysql:8.4 镜像不是必须条件，因为离线包也会包含它。
已有数据库时必须保留 /data/novro/mysql 和 /data/novro/.env.docker。

## 三、方式一：GHCR 镜像部署

GHCR 方式由构建机负责构建和推送，服务器只拉取镜像并启动，不执行 Dockerfile 构建。

### 1. 本地登录 GHCR

需要 GitHub 用户名和具有 packages:write 权限的 Token。Token 只通过标准输入使用，
不要写入脚本、Git、镜像或环境文件。

~~~powershell
$ghcrUser = '$ghcr用户$'
$ghcrToken = Read-Host 'GHCR token' -AsSecureString
$ghcrTokenPlain = [System.Net.NetworkCredential]::new('', $ghcrToken).Password
$ghcrTokenPlain | docker login ghcr.io --username $ghcrUser --password-stdin
Remove-Variable ghcrTokenPlain, ghcrToken
~~~

### 2. 构建并推送应用镜像

~~~powershell
$ghcrImage = 'ghcr.io/$ghcr用户$/$仓库名$:$版本$'
docker buildx build --platform linux/amd64 --file .\Dockerfile --tag $ghcrImage --push .
docker buildx imagetools inspect $ghcrImage
~~~

如果仓库是私有的，服务器也需要使用 packages:read 权限的 Token 登录 GHCR。

### 3. 服务器拉取镜像

~~~bash
echo '<GHCR_READ_TOKEN>' | docker login ghcr.io --username '$ghcr用户$' --password-stdin
docker pull ghcr.io/$ghcr用户$/$仓库名$:$版本$
docker pull mysql:8.4
docker image inspect ghcr.io/$ghcr用户$/$仓库名$:$版本$ --format '{{.Os}}/{{.Architecture}}'
~~~

不要把真实 Token 直接写入 shell 历史。公共包可以跳过 docker login。

服务器还需要 Compose 文件和 Nginx 模板。GHCR 部署不需要把完整源码放到服务器，可以只上传
这些部署文件：

~~~powershell
$serverIp = '$ip$'
$releaseDir = '/opt/novro/releases/<release-id>'
ssh root@$serverIp "install -d -m 0750 $releaseDir/deploy"
scp .\compose.yaml root@${serverIp}:$releaseDir/
scp .\compose.http.yaml root@${serverIp}:$releaseDir/
scp -r .\deploy\* root@${serverIp}:$releaseDir/deploy/
~~~

也可以直接把当前仓库复制或克隆到 releaseDir；GHCR 方式的关键是启动时使用 --no-build，
确保服务器不执行 Dockerfile 构建。

### 4. 创建 GHCR 运行环境

全新服务器执行下面的初始化片段。它会随机生成数据库密码、会话密钥和提供商加密密钥，
管理员密码通过隐藏输入读取：

~~~bash
DATA_DIR='/data/novro'
DOMAIN='$域名$'
NOVRO_IMAGE='ghcr.io/$ghcr用户$/$仓库名$:$版本$'
MYSQL_IMAGE='mysql:8.4'

install -d -m 0700 "$DATA_DIR"
if [ ! -f "$DATA_DIR/.env.docker" ]; then
  DB_PASSWORD="$(openssl rand -hex 24)"
  DB_ROOT_PASSWORD="$(openssl rand -hex 24)"
  SESSION_SECRET="$(openssl rand -hex 32)"
  PROVIDER_SECRET="$(openssl rand -hex 32)"

  printf 'Initial Novro administrator password: '
  read -r -s ADMIN_PASSWORD
  printf '\n'

  umask 077
  {
    printf 'MYSQL_DATABASE=novro\n'
    printf 'MYSQL_USER=novro_app\n'
    printf 'MYSQL_PASSWORD=%s\n' "$DB_PASSWORD"
    printf 'MYSQL_ROOT_PASSWORD=%s\n' "$DB_ROOT_PASSWORD"
    printf 'MYSQL_IMAGE=%s\n' "$MYSQL_IMAGE"
    printf 'NOVRO_IMAGE=%s\n' "$NOVRO_IMAGE"
    printf 'NOVRO_DATA_DIR=%s\n' "$DATA_DIR"
    printf 'NOVRO_NGINX_CONF=%s/nginx.conf\n' "$DATA_DIR"
    printf 'NOVRO_PUBLIC_URL=https://%s\n' "$DOMAIN"
    printf 'NOVRO_ALLOWED_ORIGINS=https://%s\n' "$DOMAIN"
    printf 'NOVRO_ENVIRONMENT=production\n'
    printf 'NOVRO_SESSION_SECRET=%s\n' "$SESSION_SECRET"
    printf 'NOVRO_PROVIDER_ENCRYPTION_SECRET=%s\n' "$PROVIDER_SECRET"
    printf 'NOVRO_SESSION_TTL=24h\n'
    printf 'NOVRO_SESSION_COOKIE_NAME=novro_session\n'
    printf 'NOVRO_SESSION_COOKIE_SECURE=true\n'
    printf 'NOVRO_REGISTRATION_ENABLED=true\n'
    printf 'NOVRO_REFERRAL_REWARD_BPS=1000\n'
    printf 'NOVRO_BOOTSTRAP_USERNAME=novro\n'
    printf 'NOVRO_BOOTSTRAP_EMAIL=admin@%s\n' "$DOMAIN"
    printf 'NOVRO_BOOTSTRAP_DISPLAY_NAME=Novro Administrator\n'
    printf 'NOVRO_BOOTSTRAP_PASSWORD=%s\n' "$ADMIN_PASSWORD"
    printf 'NOVRO_HTTP_PORT=80\n'
    printf 'NOVRO_HTTPS_PORT=443\n'
    printf 'NOVRO_BIND_ADDRESS=0.0.0.0\n'
    printf 'TZ=Asia/Shanghai\n'
  } > "$DATA_DIR/.env.docker"
  chmod 0600 "$DATA_DIR/.env.docker"
  unset DB_PASSWORD DB_ROOT_PASSWORD SESSION_SECRET PROVIDER_SECRET ADMIN_PASSWORD
else
  cp -a "$DATA_DIR/.env.docker" "$DATA_DIR/.env.docker.before-ghcr"
  sed -i "s|^NOVRO_IMAGE=.*|NOVRO_IMAGE=$NOVRO_IMAGE|" "$DATA_DIR/.env.docker"
  sed -i "s|^NOVRO_PUBLIC_URL=.*|NOVRO_PUBLIC_URL=https://$DOMAIN|" "$DATA_DIR/.env.docker"
  sed -i "s|^NOVRO_ALLOWED_ORIGINS=.*|NOVRO_ALLOWED_ORIGINS=https://$DOMAIN|" "$DATA_DIR/.env.docker"
  sed -i 's|^NOVRO_ENVIRONMENT=.*|NOVRO_ENVIRONMENT=production|' "$DATA_DIR/.env.docker"
  sed -i 's|^NOVRO_SESSION_COOKIE_SECURE=.*|NOVRO_SESSION_COOKIE_SECURE=true|' "$DATA_DIR/.env.docker"
  chmod 0600 "$DATA_DIR/.env.docker"
fi
~~~

已有数据库时，不要重新生成 MYSQL_PASSWORD、MYSQL_ROOT_PASSWORD、NOVRO_SESSION_SECRET 或
NOVRO_PROVIDER_ENCRYPTION_SECRET。只更新 NOVRO_IMAGE、NOVRO_PUBLIC_URL 和允许来源。

### 5. HTTPS 证书和 GHCR 启动

有可信证书时：

~~~bash
install -d -m 0700 /data/novro/tls
install -m 0644 /path/to/fullchain.pem /data/novro/tls/fullchain.pem
install -m 0600 /path/to/privkey.pem /data/novro/tls/privkey.pem
install -m 0644 /opt/novro/releases/<release-id>/deploy/nginx.conf /data/novro/nginx.conf

cd /opt/novro/releases/<release-id>
docker compose --project-directory . --env-file /data/novro/.env.docker config --quiet
docker compose --project-directory . --env-file /data/novro/.env.docker up -d --no-build --pull always
docker compose --project-directory . --env-file /data/novro/.env.docker exec -T novro nginx -t
~~~

没有可信证书时，不要调用部署脚本的非离线分支来生成证书，因为非离线分支会执行服务器本地
构建。可以先生成临时自签名证书，再使用下面的 Compose no-build 命令启动：

~~~bash
DOMAIN='$域名$'
install -d -m 0700 /data/novro/tls
openssl req -x509 -nodes -newkey rsa:3072 -sha256 -days 825 \
  -keyout /data/novro/tls/privkey.pem \
  -out /data/novro/tls/fullchain.pem \
  -subj "/CN=$DOMAIN" \
  -addext "subjectAltName=DNS:$DOMAIN" \
  -addext "keyUsage=digitalSignature,keyEncipherment" \
  -addext "extendedKeyUsage=serverAuth"
chmod 0600 /data/novro/tls/privkey.pem
chmod 0644 /data/novro/tls/fullchain.pem

docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker up -d --no-build --pull never
~~~

自签名证书只适合临时验证，浏览器会提示证书不受信任。正式环境应换成包含域名 SAN 的可信证书。

GHCR 方式的关键点：

~~~text
NOVRO_IMAGE=ghcr.io/$ghcr用户$/$仓库名$:$版本$
docker compose up -d --no-build --pull always
~~~

不要使用 docker compose up --build，否则 Compose 可能使用服务器上的 Dockerfile 重新构建。

首次启动并确认健康后，清空引导密码并只重建 Novro：

~~~bash
sed -i 's/^NOVRO_BOOTSTRAP_PASSWORD=.*/NOVRO_BOOTSTRAP_PASSWORD=/' /data/novro/.env.docker
docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker \
  up -d --force-recreate --no-deps --no-build --pull never novro
~~~

### 6. GHCR 更新

每次发布新版本时推送不可变标签：

~~~powershell
docker buildx build --platform linux/amd64 --tag 'ghcr.io/$ghcr用户$/$仓库名$:$新版本$' --push .
~~~

服务器更新：

~~~bash
sed -i 's|^NOVRO_IMAGE=.*|NOVRO_IMAGE=ghcr.io/$ghcr用户$/$仓库名$:$新版本$|' /data/novro/.env.docker
docker pull 'ghcr.io/$ghcr用户$/$仓库名$:$新版本$'
docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker \
  up -d --no-build --pull never
~~~

生产环境不要依赖 latest 作为回滚版本，优先使用版本标签或 digest。

## 四、方式二：离线包部署

离线包包含 novro:offline、mysql:8.4、Compose 文件、Nginx 配置和部署脚本。
服务器不需要访问 GHCR、Docker Hub 或 Go/Node/pnpm 下载源。

### 1. 本地打包

复用已经通过 Smoke Test 的本地镜像：

~~~powershell
powershell -ExecutionPolicy Bypass -File .\scripts\export-offline-images.ps1 -Platform linux/amd64 -ReuseExistingImages
~~~

需要重新构建时：

~~~powershell
powershell -ExecutionPolicy Bypass -File .\scripts\export-offline-images.ps1 -Platform linux/amd64
~~~

上传压缩包和校验文件：

~~~powershell
scp .\dist\novro-offline-amd64-<commit>-<time>.tar.gz root@$ip$:/root/
scp .\dist\novro-offline-amd64-<commit>-<time>.tar.gz.sha256 root@$ip$:/root/
~~~

### 2. 服务器校验和部署

~~~bash
cd /root
ARCHIVE='novro-offline-amd64-<commit>-<time>.tar.gz'
sha256sum -c "$ARCHIVE.sha256"

RELEASE_ID="$(date +%Y%m%d-%H%M%S)"
RELEASE_DIR="/opt/novro-offline/releases/$RELEASE_ID"
install -d -m 0750 "$RELEASE_DIR"
tar -xzf "/root/$ARCHIVE" -C "$RELEASE_DIR" --strip-components=1

cd "$RELEASE_DIR"
sha256sum -c SHA256SUMS
cat manifest.txt

DOMAIN='$域名$'
COMPOSE_PROJECT_NAME=novro NOVRO_DATA_DIR=/data/novro \
bash scripts/deploy-docker.sh --scheme https --domain "$DOMAIN" --offline-images ./novro-images.tar
~~~

离线脚本会执行 docker load，确认镜像为 Linux 且架构匹配，然后执行
docker compose up -d --no-build --pull never。它还会执行数据库检查、迁移、管理员初始化、
Nginx 配置和 readyz/login 检查。

没有可信证书时，脚本生成自签名证书。已有可信证书时：

~~~bash
COMPOSE_PROJECT_NAME=novro NOVRO_DATA_DIR=/data/novro \
bash scripts/deploy-docker.sh \
  --scheme https \
  --domain '$域名$' \
  --tls-cert /path/to/fullchain.pem \
  --tls-key /path/to/privkey.pem \
  --offline-images ./novro-images.tar
~~~

离线包会保留在 /root；解压和实际部署目录在 /opt/novro-offline/releases 下。

## 五、方式三：服务器本地构建

本方式把源码放到服务器，由服务器使用 Dockerfile 构建镜像。服务器必须能访问构建所需的
Debian、Go、Node.js、pnpm 等网络源，不适合隔离生产环境。

### 1. 获取源码

~~~bash
install -d -m 0750 /opt/novro/releases/<release-id>
cd /opt/novro/releases/<release-id>
git clone '$仓库地址$' .
git rev-parse HEAD
~~~

或者在本地打包源码后上传并解压。不要上传 .env 文件、私钥或真实密钥。

### 2. 构建、启动和验证

首次部署可以直接使用部署脚本：

~~~bash
cd /opt/novro/releases/<release-id>
export NOVRO_BOOTSTRAP_EMAIL='admin@$域名$'
printf 'Initial Novro administrator password: '
read -r -s NOVRO_BOOTSTRAP_PASSWORD
printf '\n'
export NOVRO_BOOTSTRAP_PASSWORD

COMPOSE_PROJECT_NAME=novro NOVRO_DATA_DIR=/data/novro \
bash scripts/deploy-docker.sh \
  --scheme https \
  --domain '$域名$' \
  --tls-cert /path/to/fullchain.pem \
  --tls-key /path/to/privkey.pem

unset NOVRO_BOOTSTRAP_PASSWORD NOVRO_BOOTSTRAP_EMAIL
~~~

没有可信证书时：

~~~bash
COMPOSE_PROJECT_NAME=novro NOVRO_DATA_DIR=/data/novro \
bash scripts/deploy-docker.sh --scheme https --domain '$域名$'
~~~

非离线模式会执行 docker compose up -d --build，服务器会运行 Dockerfile 中的构建步骤。
已有 /data/novro/.env.docker 时，脚本会保留数据库密码和应用密钥。

## 六、Nginx 配置

Nginx、Go API 和 Next.js 都在 novro 容器内部。配置源文件是：

~~~text
deploy/nginx.conf       HTTPS 配置
deploy/nginx.http.conf  HTTP 临时配置
~~~

部署脚本会把选定模板复制到：

~~~text
/data/novro/nginx.conf
~~~

Compose 只读挂载：

~~~text
/data/novro/nginx.conf -> /etc/nginx/nginx.conf:ro
/data/novro/tls         -> /etc/nginx/tls:ro
~~~

HTTPS 配置的完整核心内容如下，仓库中的 deploy/nginx.conf 是最终源文件：

~~~nginx
user www-data;
worker_processes auto;
pid /run/nginx.pid;
error_log /dev/stderr warn;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    sendfile on;
    tcp_nopush on;
    keepalive_timeout 65;
    server_tokens off;

    map $http_upgrade $connection_upgrade {
        default upgrade;
        '' close;
    }

    server {
        listen 80 default_server;
        listen [::]:80 default_server;
        server_name _;
        return 308 https://$host$request_uri;
    }

    server {
        listen 443 ssl http2 default_server;
        listen [::]:443 ssl http2 default_server;
        server_name _;

        ssl_certificate /etc/nginx/tls/fullchain.pem;
        ssl_certificate_key /etc/nginx/tls/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 1d;
        ssl_session_tickets off;

        client_max_body_size 0;
        client_body_timeout 24d;
        send_timeout 24d;

        add_header X-Content-Type-Options nosniff always;
        add_header Referrer-Policy strict-origin-when-cross-origin always;
        add_header X-Frame-Options SAMEORIGIN always;

        location = /healthz {
            proxy_pass http://127.0.0.1:8080/healthz;
            include /etc/nginx/proxy_params;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        location = /readyz {
            proxy_pass http://127.0.0.1:8080/readyz;
            include /etc/nginx/proxy_params;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        location ^~ /api/ {
            proxy_pass http://127.0.0.1:8080;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Host $host;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        location ^~ /v1/ {
            proxy_pass http://127.0.0.1:8080;
            proxy_http_version 1.1;
            proxy_buffering off;
            proxy_request_buffering off;
            proxy_cache off;
            proxy_read_timeout 24d;
            proxy_send_timeout 24d;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Host $host;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";
            add_header X-Accel-Buffering no always;
        }

        location / {
            proxy_pass http://127.0.0.1:3000;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Host $host;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
        }
    }
}
~~~

路由规则：

- /healthz、/readyz、/api/ 转发到 Go API 的 8080。
- /v1/ 转发到 Go 模型网关，关闭代理缓冲并使用长超时，支持流式响应。
- 其他路径转发到 Next.js 的 3000。
- HTTPS 证书必须位于 /data/novro/tls/fullchain.pem 和 /data/novro/tls/privkey.pem。
- 不要手工改容器内的 /etc/nginx/nginx.conf，应修改 deploy/nginx.conf 或宿主机的
  /data/novro/nginx.conf 后重建 Novro 容器。

HTTP 临时配置来自 deploy/nginx.http.conf，不包含 TLS 和 80 到 443 的跳转。HTTP 模式同步设置：

~~~text
NOVRO_PUBLIC_URL=http://$ip$
NOVRO_ALLOWED_ORIGINS=http://$ip$
NOVRO_ENVIRONMENT=development
NOVRO_SESSION_COOKIE_SECURE=false
~~~

手工更新配置或证书后检查并重建：

~~~bash
docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker exec -T novro nginx -t

docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker up -d --force-recreate --no-deps --no-build --pull never novro
~~~

## 七、通用验证和故障排查

~~~bash
docker compose --project-directory /opt/novro/releases/<release-id> \
  --env-file /data/novro/.env.docker ps

docker inspect novro-novro-1 --format '{{json .State.Health}}'
docker inspect novro-mysql-1 --format '{{json .State.Health}}'
~~~

从服务器本机验证：

~~~bash
DOMAIN='$域名$'
curl --fail --silent --show-error --insecure --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/readyz"
curl --fail --silent --show-error --insecure --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/login" -o /dev/null
~~~

预期 readyz 返回：

~~~json
{"status":"ok"}
~~~

确认 MySQL 没有公网端口：

~~~bash
docker port novro-mysql-1
docker inspect novro-mysql-1 --format '{{json .NetworkSettings.Ports}}'
~~~

如果 Nginx 返回 502，通常是 Go API 的 8080 或 Next.js 的 3000 没有启动：

~~~bash
docker logs --tail=200 novro-novro-1
docker exec novro-novro-1 nginx -t
docker exec novro-novro-1 curl --fail http://127.0.0.1:8080/readyz
docker exec novro-novro-1 curl --fail http://127.0.0.1:3000/login
~~~

如果应用无法连接 MySQL，检查 MySQL 是否 healthy、环境文件中的数据库密码是否与首次初始化时
一致，以及 MySQL 是否仍然位于 Compose 的 backend 网络。不要先删除容器或数据库目录。

确认引导密码已经从环境文件清空：

~~~bash
awk -F= '/^NOVRO_BOOTSTRAP_PASSWORD=/{print "NOVRO_BOOTSTRAP_PASSWORD_LENGTH=" length($2)}' /data/novro/.env.docker
~~~

应显示长度为 0。不要直接打印整个 .env.docker。

## 八、服务器更换 IP

如果只是服务器公网 IP 变化：

1. 把 DNS 服务商中 $域名$ 的 A 记录改成新的 $ip$。
2. 确认新服务器放行 22/tcp、80/tcp、443/tcp。
3. 保留原来的 /data/novro/.env.docker 和 /data/novro/mysql。
4. 用离线包重新执行服务器端校验、解压和部署，或用 GHCR 重新拉取固定版本镜像。
5. 从外部执行 curl -I https://$域名$/login。

证书绑定的是域名而不是 IP，单纯换 IP 不需要修改证书。只有域名本身变化时，才需要使用新域名
重新执行 --domain 并准备匹配的新证书。

## 九、禁止的破坏性操作

升级或重部署时禁止执行：

~~~bash
docker compose down -v
rm -rf /data/novro/mysql
docker volume prune
~~~

应用不健康时，先看状态和日志，不要删除容器或数据目录。
