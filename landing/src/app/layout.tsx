import type { Metadata } from "next";
import type { ReactNode } from "react";

import "./base.scss";
import "./scrollbar.scss";
import "./selection.scss";


const description = "Прогнозируйте остаток денег, планируйте платежи и заранее замечайте возможный кассовый разрыв с Saldo Analytics.";

const metadata: Metadata = {
    title: "Saldo Analytics — смотрите на финансы вперёд",
    description,
    applicationName: "Saldo Analytics",
    icons: {
        icon: "/favicons/favicon.svg",
        apple: "/favicons/apple-touch-icon.png",
    },
    openGraph: {
        title: "Saldo Analytics — смотрите на финансы вперёд",
        description,
        locale: "ru_RU",
        type: "website",
    },
};


const RootLayout = ( { children }: { children: ReactNode } ) => (
    <html lang="ru"><body>{ children }</body></html>
);


export { metadata };
export default RootLayout;
