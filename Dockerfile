# syntax=docker/dockerfile:1.7

FROM golang:1.26.5-bookworm AS api-builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY ent ./ent
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/novro ./cmd/novro

FROM node:24-bookworm-slim AS web-builder

ENV NEXT_TELEMETRY_DISABLED=1 \
    NOVRO_SERVER_URL=http://127.0.0.1:8080 \
    PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH

WORKDIR /workspace
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
RUN pnpm install --frozen-lockfile
COPY apps/web ./apps/web
RUN pnpm install --frozen-lockfile
RUN pnpm --dir apps/web build

FROM node:24-bookworm-slim AS runtime

ENV NEXT_TELEMETRY_DISABLED=1 \
    NODE_ENV=production \
    NOVRO_SERVER_URL=http://127.0.0.1:8080

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl nginx supervisor \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system novro \
    && useradd --system --gid novro --home-dir /home/novro --create-home novro

COPY --from=api-builder /out/novro /usr/local/bin/novro
COPY --from=web-builder /workspace/apps/web/.next/standalone /opt/novro/web
COPY --from=web-builder /workspace/apps/web/.next/static /opt/novro/web/apps/web/.next/static
COPY --from=web-builder /workspace/apps/web/public /opt/novro/web/apps/web/public
COPY deploy/nginx.conf /etc/nginx/nginx.conf
COPY deploy/supervisord.conf /etc/supervisor/supervisord.conf
COPY deploy/docker-entrypoint.sh /usr/local/bin/novro-entrypoint

RUN chmod 0755 /usr/local/bin/novro /usr/local/bin/novro-entrypoint \
    && chown -R novro:novro /opt/novro /home/novro \
    && mkdir -p /run/nginx /var/log/supervisor

EXPOSE 80 443

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD /bin/sh -ceu 'curl --fail --silent --show-error http://127.0.0.1:8080/readyz >/dev/null && case "${NOVRO_PUBLIC_URL:-https://localhost}" in http://*) curl --fail --silent --show-error http://127.0.0.1/login >/dev/null ;; https://*) curl --fail --silent --show-error --insecure https://127.0.0.1/login >/dev/null ;; *) exit 1 ;; esac'

ENTRYPOINT ["/usr/local/bin/novro-entrypoint"]
CMD ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisor/supervisord.conf"]
