import type { NextConfig } from "next";


const appUrl = (process.env.NEXT_PUBLIC_APP_URL || "https://saldo.brokebyte.me").replace(/\/$/, "");

const nextConfig: NextConfig = {
    output: "standalone",
    basePath: "/landing",
    agentRules: false,
    experimental: { useTypeScriptCli: false },
    redirects: async () => [
        { source: "/privacy-policy", destination: `${appUrl}/privacy-policy`, permanent: false },
        { source: "/personal-data-consent", destination: `${appUrl}/personal-data-consent`, permanent: false },
    ],
};


export default nextConfig;
