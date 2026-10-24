"use client";

import { useRef, useState } from "react";
import { useSelector } from "react-redux";

import { authService } from "@/service";
import type { RootState } from "@/store/store";
import { ActionButton, LinkButton } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


const Account = () => {
    const user = useSelector(( state: RootState ) => state.auth.user);
    const [ busy, setBusy ] = useState(false);
    const pending = useRef(false);


    const logout = async () => {
        if (pending.current) {
            return;
        }

        pending.current = true;
        setBusy(true);

        try {
            await authService.logout();
            window.location.replace("/login");
        } catch {
            pending.current = false;
            setBusy(false);
        }
    };


    return (
        <main className={styles.page}>
            <section className={styles.card} aria-labelledby="account-title">
                <h1 id="account-title">Личный кабинет</h1>
                <p>{ user?.display_name }</p>
                <p>{ user?.email }</p>
                <div className={styles.actions}>
                    <LinkButton href="/workspace" icon="building">Мои компании</LinkButton>
                    <ActionButton type="button" icon="logout" loading={busy} onClick={logout}>Выйти</ActionButton>
                </div>
            </section>
        </main>
    );
};


export default Account;
