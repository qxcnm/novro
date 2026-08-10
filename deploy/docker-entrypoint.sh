#!/bin/sh
set -eu

case "${NOVRO_PUBLIC_URL:-https://localhost}" in
    https://*)
        if [ ! -r /etc/nginx/tls/fullchain.pem ] || [ ! -r /etc/nginx/tls/privkey.pem ]; then
            echo "TLS certificate files are missing from /etc/nginx/tls" >&2
            exit 1
        fi
        ;;
    http://*)
        ;;
    *)
        echo "NOVRO_PUBLIC_URL must start with http:// or https://" >&2
        exit 1
        ;;
esac

nginx -t

attempt=1
until /usr/local/bin/novro check-db; do
    if [ "$attempt" -ge 60 ]; then
        echo "MySQL did not become ready after 120 seconds" >&2
        exit 1
    fi
    attempt=$((attempt + 1))
    sleep 2
done

/usr/local/bin/novro migrate

if [ -n "${NOVRO_BOOTSTRAP_PASSWORD:-}" ]; then
    /usr/local/bin/novro bootstrap-admin
fi

exec "$@"
