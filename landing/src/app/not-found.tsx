import Link from "next/link";

import styles from "./not-found.module.scss";


const appUrl = (process.env.NEXT_PUBLIC_APP_URL || "https://saldo.brokebyte.me").replace(/\/$/, "");


const NotFound = () => (
    <main className={styles.page}>
        <section className={styles.card} aria-labelledby="not-found-title">
            <div className={styles.visual} aria-hidden="true"><span>4</span><span className={styles.zero}>0</span><span>4</span><svg viewBox="0 0 320 90" fill="none"><path d="M0 70L55 52L102 64L151 24L205 40L260 18L320 4" /></svg></div>
            <span className={styles.eyebrow}>Немного сбились с курса</span>
            <h1 id="not-found-title">Страница не найдена</h1>
            <p>Возможно, ссылка устарела или в адресе появилась опечатка. Вернёмся туда, где всё на своих местах.</p>
            <div className={styles.actions}><Link className={styles.primary} href="/">На главную <span aria-hidden="true">↗</span></Link><Link className={styles.secondary} href={`${appUrl}/login`}>Войти в Saldo</Link></div>
        </section>
    </main>
);


export default NotFound;
