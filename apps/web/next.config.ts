import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    const backend = process.env.NOVRO_SERVER_URL ?? "http://127.0.0.1:8080";
    return [{
      source: "/api/:path*",
      destination: `${backend}/api/:path*`,
    }];
  },
};

export default nextConfig;
