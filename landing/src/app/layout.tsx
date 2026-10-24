import type { Metadata } from "next";
import type { ReactNode } from "react";

import "./base.scss";
import "./scrollbar.scss";
import "./selection.scss";


const description = "Saldo Analytics помогает прогнозировать денежные потоки бизнеса, выявлять кассовые разрывы, планировать платежи и сравнивать финансовые сценарии.";


const metadata: Metadata = {
    applicationName: "Saldo Analytics",
    title: {
        default: "Saldo Analytics — финансовый прогноз для бизнеса",
        template: "%s | Saldo Analytics",
    },
    description,
    keywords: [ "Saldo", "Saldo Analytics", "финансовое прогнозирование", "денежные потоки", "кассовые разрывы", "платёжный календарь", "план-факт", "финансы малого бизнеса" ],
    manifest: "/landing/favicons/site.webmanifest",
    icons: {
        icon: [
            { url: "/landing/favicons/favicon.svg", type: "image/svg+xml" },
            { url: "/landing/favicons/favicon-32.png", sizes: "32x32", type: "image/png" },
            { url: "/landing/favicons/favicon-16.png", sizes: "16x16", type: "image/png" },
        ],
        shortcut: "/landing/favicons/favicon.ico",
        apple: "/landing/favicons/apple-touch-icon.png",
    },
    openGraph: {
        title: "Saldo Analytics — финансовый прогноз для бизнеса",
        description,
        siteName: "Saldo Analytics",
        locale: "ru_RU",
        type: "website",
    },
    twitter: {
        card: "summary",
        title: "Saldo Analytics — финансовый прогноз для бизнеса",
        description,
    },
};


const RootLayout = ( { children }: { children: ReactNode } ) => (
    <html lang="ru"><body>{ children }</body></html>
);


export { metadata };
export default RootLayout;
