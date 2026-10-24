import type { NextConfig } from "next";


const pagesBuild = process.env.PAGES_BUILD === "true";


const nextConfig: NextConfig = {
    output: pagesBuild ? "export" : "standalone",
    basePath: process.env.NEXT_PUBLIC_BASE_PATH || "",
    trailingSlash: pagesBuild,
    images: {
        unoptimized: pagesBuild,
    },
    experimental: {
        useTypeScriptCli: false,
    },
    env: {
        developerMode: pagesBuild ? "mock" : process.env.developerMode || "dev",
    },
};


export default nextConfig;
