import type { NextConfig } from "next";

const defaultBackendURL = "http://127.0.0.1:8080";

/**
 * normalizeBackendURL 校验并规范化后端服务地址。
 * @param value 待校验的后端服务地址。
 * @param production 指示当前是否使用生产环境校验规则。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function normalizeBackendURL(value: string | undefined, production = process.env.NODE_ENV === "production") {
  const configured = (value ?? defaultBackendURL).trim();
  let backend: URL;
  try {
    backend = new URL(configured);
  } catch {
    throw new Error("NOVRO_SERVER_URL must be an absolute http or https origin without credentials, path, query, or fragment");
  }
  if ((backend.protocol !== "http:" && backend.protocol !== "https:") || backend.username || backend.password || backend.pathname !== "/" || backend.search || backend.hash) {
    throw new Error("NOVRO_SERVER_URL must be an absolute http or https origin without credentials, path, query, or fragment");
  }
  if (production && !isLoopbackHostname(backend.hostname)) {
    throw new Error("production NOVRO_SERVER_URL must use a loopback host");
  }
  return backend.origin;
}

/**
 * isLoopbackHostname 判断主机名是否指向本机回环地址。
 * @param hostname 待检查的主机名。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function isLoopbackHostname(hostname: string) {
  const normalized = hostname.toLowerCase();
  if (normalized === "localhost" || normalized === "[::1]") {
    return true;
  }
  const octets = normalized.split(".");
  return octets.length === 4 && octets[0] === "127" && octets.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

const nextConfig: NextConfig = {
  output: "standalone",
  /**
   * rewrites 构建前端 API 到 Go 服务的代理重写规则。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async rewrites() {
    const backend = normalizeBackendURL(process.env.NOVRO_SERVER_URL);
    return [{
      source: "/api/:path*",
      destination: `${backend}/api/:path*`,
    }];
  },
};

export default nextConfig;
