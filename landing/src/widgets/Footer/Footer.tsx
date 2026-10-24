"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";

import { apps, bankUrl, contactLinks, groups, legalLinks, socialLinks } from "./links";
import styles from "./style.module.scss";


const Footer = () => {
    const [ expanded, setExpanded ] = useState<string[]>([]);
    const [ phonesOpen, setPhonesOpen ] = useState(false);


    const toggleGroup = ( title: string ) => {
        setExpanded(( current ) => current.includes(title) ? current.filter(( item ) => item !== title) : [ ...current, title ]);
    };


    return (
        <footer className={styles.footer}>
            <div className={styles.container}>
                <div className={styles.top}>
                    <div className={styles.phones}>
                        <div className={styles.phone}>
                            <span>Бесплатный звонок по России</span>
                            <a href="tel:88002009929">8 800 200 99 29</a>
                        </div>
                        <button className={styles.morePhones} type="button" aria-expanded={phonesOpen} aria-controls="footer-additional-phones" onClick={() => setPhonesOpen(!phonesOpen)}>
                            Дополнительные номера<span className={styles.arrow} aria-hidden="true" />
                        </button>
                        <div id="footer-additional-phones" className={`${styles.additionalPhones} ${phonesOpen ? styles.expanded : ""}`}>
                            <div className={styles.phone}>
                                <span>Поддержка по картам и банкоматам</span>
                                <a href="tel:88002009109">8 800 200 91 09</a>
                            </div>
                            <div className={styles.phone}>
                                <span>Для звонков из других стран</span>
                                <a href="tel:+78632000000">+7 863 200 00 00</a>
                            </div>
                        </div>
                    </div>

                    <nav className={styles.navigation} aria-label="Разделы сайта банка">
                        { groups.map(( group, index ) => (
                            <section key={group.title} className={`${styles.group} ${styles[`group${index}`]}`}>
                                <div className={styles.groupHeading}>
                                    <h2>
                                        <a href={bankUrl(group.href)} aria-label={group.title === "ESG" ? "ESG — устойчивое развитие" : undefined}>
                                            { group.title === "ESG" ? <Image src="/landing/icons/bank/footer/green-vector.svg" alt="" width={34} height={34} unoptimized /> : group.title }
                                        </a>
                                    </h2>
                                    <button type="button" className={styles.toggle} aria-label={`${expanded.includes(group.title) ? "Свернуть" : "Развернуть"} раздел «${group.title}»`} aria-expanded={expanded.includes(group.title)} aria-controls={`footer-group-${index}`} onClick={() => toggleGroup(group.title)}>
                                        <span className={styles.arrow} aria-hidden="true" />
                                    </button>
                                </div>
                                <ul id={`footer-group-${index}`} className={expanded.includes(group.title) ? styles.expanded : ""}>
                                    { group.links.map(( link ) => <li key={link.title}><a href={bankUrl(link.href)}>{ link.title }</a></li>) }
                                </ul>
                            </section>
                        )) }
                        <div className={styles.social}>
                            <section>
                                <h2>Следить за нами</h2>
                                <ul className={styles.socialLinks}>
                                    { socialLinks.map(( link, index ) => (
                                        <li key={link.href} className={index > 3 ? styles.telegram : undefined}>
                                            <a href={link.href} aria-label={index < 4 ? link.title : undefined}>
                                                <Image src={`/landing/icons/bank/footer/${link.icon}.svg`} alt="" width={link.icon === "telegram" ? 21 : 24} height={link.icon === "vk" ? 15 : link.icon === "telegram" ? 17 : 24} unoptimized />{ index > 3 && <span>{ link.title }</span> }
                                            </a>
                                        </li>
                                    )) }
                                </ul>
                            </section>
                            <section>
                                <h2>Связаться с нами</h2>
                                <ul className={styles.contactLinks}>
                                    { contactLinks.map(( link ) => <li key={link.href}><a href={bankUrl(link.href)} aria-label={link.title}><Image src={`/landing/icons/bank/footer/${link.icon}.svg`} alt="" width={link.icon === "internet-reception" ? 21 : 24} height={link.icon === "vk" ? 15 : link.icon === "internet-reception" ? 21 : 24} unoptimized /></a></li>) }
                                </ul>
                            </section>
                        </div>
                    </nav>

                    <div className={styles.online}>
                        <a className={styles.onlineButton} href={bankUrl("/for-individuals/online-applications")}>Онлайн - сервисы</a>
                        <ul className={styles.apps}>
                            { apps.map(( app ) => <li key={app.title}><a href={bankUrl(app.href)} aria-label={app.title}><Image src={`/landing/icons/bank/footer/${app.icon}.svg`} alt="" width={74} height={74} unoptimized /></a></li>) }
                        </ul>
                    </div>
                </div>

                <div className={styles.legal}>
                    <div className={styles.legalRow}>
                        { legalLinks.slice(0, 6).map(( link ) => <a key={link.title} href={bankUrl(link.href)}>{ link.title }</a>) }
                    </div>
                    <div className={styles.legalRow}>
                        <p>Уставный капитал — 933 567 570,00 руб., дата изменения величины уставного капитала: 29.10.2015</p>
                        { legalLinks.slice(6, 8).map(( link ) => <a key={link.title} href={bankUrl(link.href)}>{ link.title }</a>) }
                    </div>
                    <div className={`${styles.legalRow} ${styles.disclosures}`}>
                        { legalLinks.slice(8).map(( link ) => <a key={link.title} href={bankUrl(link.href)}>{ link.title }</a>) }
                    </div>
                    <p className={styles.cookies}>Для работы сервиса используются необходимые файлы cookie. Подробнее об обработке данных — в <Link href="/privacy-policy">политике конфиденциальности</Link> и <Link href="/personal-data-consent">согласии на обработку персональных данных</Link>.</p>
                    <p>Copyright © 1997-{ new Date().getFullYear() }, ПАО КБ «Центр-инвест», 344000, г. Ростов-на-Дону, пр. Соколова, 62</p>
                    <p className={styles.credit}>Design&amp;Support © <a href="https://cinet.ru/" title="Разработчик официального сайта банка Центр-инвест">«Центр-Интернет»</a></p>
                </div>
            </div>
        </footer>
    );
};


export default Footer;
