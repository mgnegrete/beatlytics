import type { NextConfig } from "next";

/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    return process.env.NODE_ENV === 'development'
      ? [
          {
            source: '/api/:path*',
            destination: 'http://localhost:8081/api/:path*',
          },
        ]
      : [];
  },
};
module.exports = nextConfig;


export default nextConfig;
