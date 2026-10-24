"use client";

import Image from "next/image";
import { useEffect, useRef, useState } from "react";

import appLinks from "@/helpers/appLinks";
import Icon from "@/widgets/Icon/Icon";

import styles from "./style.module.scss";


const navigation = [
    { href: "#features", label: "Возможности" },
    { href: "#how-it-works", label: "Как это работает" },
    { href: "#scenarios", label: "Сценарии" },
    { href: "#faq", label: "Вопросы" },
];


const Header = () => {
    const [ open, setOpen ] = useState(false);
    const toggleRef = useRef<HTMLButtonElement>(null);

    useEffect(() => {
        if (!open) {
            return;
        }

        const closeOnEscape = ( event: KeyboardEvent ) => {
            if (event.key === "Escape") {
                setOpen(false);
                toggleRef.current?.focus();
            }
        };

        document.addEventListener("keydown", closeOnEscape);

        return () => document.removeEventListener("keydown", closeOnEscape);
    }, [ open ]);

    return (
        <header className={styles.header}>
            <div className={styles.headerInner}>
                <a href="#" className={styles.brand} aria-label="Saldo Analytics — на главную">
                    <Image src="/icons/saldo/saldo-analytics-black.svg" alt="Saldo Analytics" width={156} height={58} priority unoptimized />
                </a>
                <nav className={styles.desktopNav} aria-label="Основная навигация">
                    { navigation.map(( item ) => <a href={item.href} key={item.href}>{ item.label }</a>) }
                </nav>
                <div className={styles.headerActions}>
                    <a href={appLinks.login} className={styles.login}>Войти <Icon name="arrow-right" size={17} /></a>
                    <a href={appLinks.register} className={styles.headerCta}>Начать работу</a>
                    <button ref={toggleRef} type="button" className={styles.menuToggle} aria-label={open ? "Закрыть меню" : "Открыть меню"} aria-expanded={open} aria-controls="mobile-menu" onClick={() => setOpen(!open)}>
                        <Icon name={open ? "x" : "adjustments"} size={23} />
                    </button>
                </div>
            </div>
            { open && <nav id="mobile-menu" className={styles.mobileNav} aria-label="Мобильная навигация">{ navigation.map(( item ) => <a href={item.href} key={item.href} onClick={() => setOpen(false)}>{ item.label }<Icon name="arrow-right" size={18} /></a>) }<a href={appLinks.register}>Начать работу<Icon name="arrow-right" size={18} /></a></nav> }
        </header>
    );
};


export default Header;

