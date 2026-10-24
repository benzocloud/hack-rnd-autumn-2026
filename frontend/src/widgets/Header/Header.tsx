"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSelector } from "react-redux";

import { isAuthRoute } from "@/helpers/auth";
import { withBasePath } from "@/helpers/basePath";
import type { ICompany } from "@/interfaces";
import type { RootState } from "@/store/store";

import styles from "./style.module.scss";


interface IProps {
    company?: Pick<ICompany, "name"> | null;
    avatarUrl?: string | null;
}


const Header = ( { company, avatarUrl }: IProps ) => {
    const pathname = usePathname();
    const selectedCompany = useSelector(( state: RootState ) => state.company?.selected || null);
    const companyName = (company || selectedCompany)?.name.trim() || "Компания";
    const initial = Array.from(companyName)[0].toLocaleUpperCase("ru-RU");


    if (isAuthRoute(pathname)) {
        return null;
    }

    return (
        <header className={styles.header}>
            <div className={styles.container}>
                <Link className={styles.bank} href="/" aria-label="Банк Центр-инвест — на главную">
                    <picture>
                        <source media="(max-width: 480px)" srcSet={withBasePath("/icons/bank/invest-bank-symbol.svg")} />
                        <Image
                            className={styles.bankLogo}
                            src={withBasePath("/icons/bank/invest-bank.svg")}
                            alt="Банк Центр-инвест"
                            width={254}
                            height={53}
                            loading="eager"
                            unoptimized
                        />
                    </picture>
                </Link>

                <Link className={styles.company} href="/account" aria-label={`Личный кабинет компании «${companyName}»`}>
                    <span className={styles.companyName} title={companyName}>{ companyName }</span>

                    <div className={styles.avatar}>
                        { avatarUrl ? (
                            <Image
                                className={styles.avatarImage}
                                src={avatarUrl}
                                alt={`Аватар компании «${companyName}»`}
                                width={45}
                                height={45}
                                unoptimized
                            />
                        ) : (
                            <span className={styles.initial} aria-hidden="true">{ initial }</span>
                        ) }
                    </div>
                </Link>
            </div>
        </header>
    );
};


export default Header;
