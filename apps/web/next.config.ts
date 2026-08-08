import type { NextConfig } from "next";

const defaultBackendURL = "http://127.0.0.1:8080";

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
  async rewrites() {
    const backend = normalizeBackendURL(process.env.NOVRO_SERVER_URL);
    return [{
      source: "/api/:path*",
      destination: `${backend}/api/:path*`,
    }];
  },
};

export default nextConfig;
