"use client";

import axios from "axios";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { useDispatch, useSelector } from "react-redux";

import { canVerifyEmail, isAuthRoute, isKnownRoute, isPublicRoute, redirectToLogin } from "@/helpers/auth";
import { accountService } from "@/service";
import { setAuthStatus, setAuthUser } from "@/store/authSlice";
import type { AppDispatch, RootState } from "@/store/store";
import { Spinner } from "@/widgets";

import styles from "./style.module.scss";


interface IProps {
    children: ReactNode;
}


const AuthBoundary = ( { children }: IProps ) => {
    const user = useSelector(( state: RootState ) => state.auth.user);

    const dispatch = useDispatch<AppDispatch>();
    const pathname = usePathname();
    const publicPage = isPublicRoute(pathname) || !isKnownRoute(pathname);
    const authPage = isAuthRoute(pathname);
    const accountPage = pathname.replace(/\/+$/, "") === "/account";
    const router = useRouter();
    const [ checkedPath, setCheckedPath ] = useState<string | null>(null);


    useEffect(() => {
        if (publicPage && !authPage) {
            return;
        }

        if (user) {
            if (!authPage && !accountPage && !user.email_verified) {
                router.replace("/verify-code");
            } else if (authPage && !canVerifyEmail(pathname, user)) {
                router.replace("/");
            }

            return;
        }

        const controller = new AbortController();

        dispatch(setAuthStatus("checking"));

        const request = authPage ? accountService.checkSession(controller.signal) : accountService.getMe(controller.signal);

        request.then(( response ) => {
            if (!controller.signal.aborted) {
                dispatch(setAuthUser(response));
            }
        }).catch(( error: unknown ) => {
            if (axios.isCancel(error) || controller.signal.aborted) {
                return;
            }

            if (authPage) {
                setCheckedPath(pathname);
            } else {
                redirectToLogin();
            }
        });

        return () => controller.abort();
    }, [ publicPage, authPage, accountPage, user, pathname, dispatch, router ]);


    if ((publicPage && !authPage) || (authPage ? (!user && checkedPath === pathname) || canVerifyEmail(pathname, user) : user && (user.email_verified || accountPage))) {
        return children;
    }

    return (
        <main className={styles.status} aria-busy="true">
            <div className={styles.message}>
                <Spinner label="Проверяем вход" />
                <p>Проверяем вход</p>
            </div>
        </main>
    );
};


export default AuthBoundary;
