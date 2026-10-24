import type { NextConfig } from "next";


const nextConfig: NextConfig = {
    output: "standalone",
    experimental: {
        useTypeScriptCli: false,
    },
    env: {
        developerMode: process.env.developerMode || "dev",
    },
};


export default nextConfig;
