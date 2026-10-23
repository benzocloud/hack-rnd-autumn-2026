import "server-only";

import Link from "next/link";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import consentDocument from "@/content/personal-data-consent.json";
import privacyPolicy from "@/content/privacy-policy.json";

import styles from "./style.module.scss";


interface IProps {
    kind: "consent" | "privacy";
}


const documentLinks: Record<string, string> = {
    "privacy-policy.md": "/privacy-policy",
    "personal-data-consent.md": "/personal-data-consent",
};


const components: Components = {
    a: ( { href, children } ) => {
        const url = href ? documentLinks[href] || href : "";

        if (!url) {
            return <span>{ children }</span>;
        }

        return url.startsWith("/") ? <Link href={url}>{ children }</Link> : <a href={url} target="_blank" rel="noopener noreferrer">{ children }</a>;
    },
    table: ( { children } ) => <div className={styles.table}><table>{ children }</table></div>,
};


const LegalDocument = ( { kind }: IProps ) => {
    const isConsent = kind === "consent";
    const text = isConsent ? consentDocument.text : privacyPolicy.text;


    return (
        <main className={styles.page}>
            <article className={styles.card}>
                <div className={styles.document}>
                    <Markdown remarkPlugins={[ remarkGfm ]} components={components}>{ text }</Markdown>
                </div>

                <nav className={styles.links} aria-label="Документы и регистрация">
                    <Link href={isConsent ? "/privacy-policy" : "/personal-data-consent"}>{ isConsent ? "Политика конфиденциальности" : "Согласие на обработку данных" }</Link>
                    <Link href="/register">Вернуться к регистрации</Link>
                </nav>
            </article>
        </main>
    );
};


export default LegalDocument;
