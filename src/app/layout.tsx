import type { Metadata } from "next";
import type { ReactNode } from "react";

import { AuthBoundary } from "@/features";
import StoreProvider from "@/store/StoreProvider";
import { Header } from "@/widgets";

import "./base.css";
import "./scrollbar.scss";
import styles from "./layout.module.scss";


interface IProps {
    children: ReactNode;
}


const description = "Saldo Analytics помогает прогнозировать денежные потоки бизнеса, выявлять кассовые разрывы, планировать платежи и сравнивать финансовые сценарии.";


const metadata: Metadata = {
    applicationName: "Saldo Analytics",
    title: {
        default: "Saldo Analytics — финансовый прогноз для бизнеса",
        template: "%s | Saldo Analytics",
    },
    description,
    keywords: [ "Saldo", "Saldo Analytics", "финансовое прогнозирование", "денежные потоки", "кассовые разрывы", "платёжный календарь", "план-факт", "финансы малого бизнеса" ],
    manifest: "/favicons/site.webmanifest",
    icons: {
        icon: [
            { url: "/favicons/favicon.svg", type: "image/svg+xml" },
            { url: "/favicons/favicon-32.png", sizes: "32x32", type: "image/png" },
            { url: "/favicons/favicon-16.png", sizes: "16x16", type: "image/png" },
        ],
        shortcut: "/favicons/favicon.ico",
        apple: "/favicons/apple-touch-icon.png",
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


const RootLayout = ( { children }: IProps ) => {
    return (
        <html className={styles.root} lang="ru">
            <body className={styles.body}>
                <StoreProvider>
                    <AuthBoundary>
                        <Header />
                        { children }
                    </AuthBoundary>
                </StoreProvider>
            </body>
        </html>
    );
};


export { metadata };
export default RootLayout;
