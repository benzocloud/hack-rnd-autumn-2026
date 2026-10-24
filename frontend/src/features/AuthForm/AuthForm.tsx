"use client";

import axios from "axios";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useDispatch, useSelector } from "react-redux";

import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar, showSuccessSnackbar } from "@/helpers/snackbar";
import validateAuthForm from "@/helpers/validateAuthForm";
import type { AuthMode, IPersonalDataConsentDocument, VerificationPurpose } from "@/interfaces";
import { accountService, authService } from "@/service";
import { clearAuthUser, setAuthUser, setVerificationEmail } from "@/store/authSlice";
import { setSelectedCompany } from "@/store/companySlice";
import type { AppDispatch, RootState } from "@/store/store";
import { AuthButton, AuthCheckbox, AuthInput, CodeInput, Spinner } from "@/widgets";

import styles from "./style.module.scss";


type IProps = ({
    mode: "register";
    consentDocument: Pick<IPersonalDataConsentDocument, "document_version" | "document_sha256">;
} | {
    mode: Exclude<AuthMode, "register">;
    consentDocument?: never;
}) & {
    verificationPurpose?: VerificationPurpose;
    resetToken?: string;
};


const titles: Record<AuthMode, string> = {
    login: "Вход в личный кабинет",
    register: "Регистрация",
    "verify-code": "Введите код подтверждения",
    "forgot-password": "Восстановление пароля",
    "reset-password": "Новый пароль",
};

const buttons: Record<AuthMode, string> = {
    login: "Войти",
    register: "Зарегистрироваться",
    "verify-code": "Подтвердить",
    "forgot-password": "Отправить письмо",
    "reset-password": "Сохранить пароль",
};


const AuthForm = ( { mode, consentDocument, verificationPurpose = "register", resetToken = "" }: IProps ) => {
    const [ busy, setBusy ] = useState(false);
    const [ resendBusy, setResendBusy ] = useState(false);
    const verificationEmail = useSelector(( state: RootState ) => state.auth.verificationEmail);

    const router = useRouter();
    const dispatch = useDispatch<AppDispatch>();
    const emailRef = useRef<HTMLInputElement>(null);
    const rememberRef = useRef<HTMLInputElement>(null);
    const consentRef = useRef<HTMLInputElement>(null);
    const requestRef = useRef<AbortController | null>(null);
    const resendRef = useRef<AbortController | null>(null);
    const isRegister = mode === "register";
    const isLogin = mode === "login";
    const isCode = mode === "verify-code";
    const isReset = mode === "reset-password";
    const codeTitles: Record<VerificationPurpose, string> = { register: "Подтверждение регистрации", login: "Подтверждение входа", reset: "Подтверждение сброса пароля" };
    const codeBackUrl = verificationPurpose === "register" ? "/register" : verificationPurpose === "reset" ? "/forgot-password" : "/login";


    useEffect(() => {
        if (isLogin) {
            try {
                const savedEmail = localStorage.getItem("saldo-remembered-email");

                if (savedEmail && emailRef.current && rememberRef.current) {
                    emailRef.current.value = savedEmail;
                    rememberRef.current.checked = true;
                }
            } catch {}
        }

        return () => {
            requestRef.current?.abort();
            resendRef.current?.abort();
        };
    }, [ isLogin ]);


    const submit = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();

        if (requestRef.current) {
            return;
        }

        const form = new FormData(event.currentTarget);
        const email = String(form.get("email") || "").trim();
        const password = String(form.get("password") || "");
        const displayName = [ form.get("first_name"), form.get("last_name") ].map(( value ) => String(value || "").trim()).join(" ").trim();

        const validationError = validateAuthForm(mode, form);

        if (validationError) {
            showErrorSnackbar(validationError);

            return;
        }

        if (isReset && (!resetToken || (process.env.developerMode !== "mock" && (resetToken.length < 16 || resetToken.length > 512)))) {
            showErrorSnackbar("Ссылка восстановления недействительна. Запросите новую.");

            return;
        }

        const controller = new AbortController();

        requestRef.current = controller;
        setBusy(true);

        try {
            if (isLogin) {
                const response = await authService.login({ data: { email, password } }, controller.signal);

                try {
                    if (rememberRef.current?.checked) {
                        localStorage.setItem("saldo-remembered-email", email);
                    } else {
                        localStorage.removeItem("saldo-remembered-email");
                    }
                } catch {}

                dispatch(setSelectedCompany(null));

                if (process.env.developerMode === "mock") {
                    dispatch(clearAuthUser());
                    dispatch(setVerificationEmail(email || response.user.email));
                    showSuccessSnackbar("Для подтверждения входа введите любые шесть цифр.");
                    router.push("/verify-code?purpose=login");
                } else {
                    dispatch(setAuthUser(response.user));
                    router.replace("/");
                }
            } else if (isRegister) {
                const response = await authService.register({
                    data: {
                        email,
                        password,
                        display_name: displayName,
                        personal_data_consent: {
                            accepted: true,
                            document_version: consentDocument.document_version,
                            document_sha256: consentDocument.document_sha256,
                        },
                    },
                }, controller.signal);

                dispatch(setVerificationEmail(email));
                showSuccessSnackbar(response.message);
                router.push("/verify-code");
            } else if (isCode) {
                const token = String(form.get("token") || "").trim();

                if (verificationPurpose === "login") {
                    const response = await authService.confirmMockLogin({ data: { token } }, controller.signal);

                    dispatch(setAuthUser(response.user));
                    showSuccessSnackbar("Вход подтверждён.");
                    router.replace("/");
                } else if (verificationPurpose === "reset") {
                    const response = await authService.confirmMockPasswordReset({ data: { token } }, controller.signal);

                    showSuccessSnackbar(response.message);
                    router.replace(`/reset-password?token=${encodeURIComponent(token)}`);
                } else {
                    const response = await authService.verifyEmail({ data: { token } }, controller.signal);
                    const user = await accountService.getMe(controller.signal);

                    dispatch(setAuthUser(user));
                    showSuccessSnackbar(response.message);
                    router.replace("/");
                }
            } else if (mode === "forgot-password") {
                const response = await authService.forgotPassword({ data: { email } }, controller.signal);

                showSuccessSnackbar(response.message);

                if (process.env.developerMode === "mock") {
                    dispatch(setVerificationEmail(email));
                    router.push("/verify-code?purpose=reset");
                }
            } else if (isReset) {
                const response = await authService.resetPassword({ data: { token: resetToken, new_password: password } }, controller.signal);

                dispatch(clearAuthUser());
                showSuccessSnackbar(response.message);
                router.replace("/login");
            }
        } catch (error: unknown) {
            if (isCode && verificationPurpose === "register" && axios.isAxiosError(error) && error.response?.status === 401) {
                router.replace("/login");
            }

            if (!axios.isAxiosError(error) && !axios.isCancel(error)) {
                showErrorSnackbar(getErrorMessage(error));
            }

            if (isRegister && axios.isAxiosError(error) && error.response?.data?.error?.code === "CONSENT_DOCUMENT_CHANGED") {
                if (consentRef.current) {
                    consentRef.current.checked = false;
                }
            }
        } finally {
            requestRef.current = null;
            setBusy(false);
        }
    };


    const resendCode = async () => {
        if (resendRef.current || !verificationEmail) {
            return;
        }

        const controller = new AbortController();

        resendRef.current = controller;
        setResendBusy(true);

        try {
            const response = verificationPurpose === "reset"
                ? await authService.forgotPassword({ data: { email: verificationEmail } }, controller.signal)
                : await authService.resendVerification({ data: { email: verificationEmail } }, controller.signal);

            showSuccessSnackbar(response.message);
        } catch {} finally {
            resendRef.current = null;
            setResendBusy(false);
        }
    };


    return (
        <main className={styles.page}>
            <section className={`${styles.card} ${isLogin ? styles.login : ""} ${isCode ? styles.code : ""}`} aria-labelledby="auth-title">
                <h1 className={styles.title} id="auth-title">{ isCode ? codeTitles[verificationPurpose] : titles[mode] }</h1>

                { mode === "forgot-password" && <p className={styles.description}>Укажите почту, чтобы получить письмо для восстановления пароля.</p> }

                <form className={styles.form} onSubmit={submit} noValidate aria-busy={busy}>
                    <fieldset className={styles.fields} disabled={busy}>
                        <legend className={styles.hidden}>Данные аккаунта</legend>

                        { isRegister && (
                            <>
                                <AuthInput showLabel name="first_name" label="Имя" autoComplete="given-name" />
                                <AuthInput showLabel name="last_name" label="Фамилия" autoComplete="family-name" />
                            </>
                        ) }

                        { !isCode && !isReset && <AuthInput showLabel={isLogin || isRegister} ref={emailRef} name="email" label="Почта" type={isLogin ? "text" : "email"} inputMode="email" autoComplete="email" autoCapitalize="none" spellCheck={false} /> }

                        { (isLogin || isRegister || isReset) && (
                            <AuthInput showLabel={isLogin || isRegister} name="password" label={isReset ? "Новый пароль" : "Пароль"} type="password" autoComplete={isRegister || isReset ? "new-password" : "current-password"} />
                        ) }

                        { (isRegister || isReset) && <AuthInput showLabel={isRegister} name="confirm_password" label="Повторите пароль" type="password" autoComplete="new-password" /> }
                        { isCode && (process.env.developerMode === "mock" ? <CodeInput /> : <AuthInput name="token" label="Токен подтверждения из письма" placeholder="0000" type="text" autoComplete="one-time-code" />) }
                    </fieldset>

                    { isLogin && (
                        <AuthCheckbox ref={rememberRef} name="remember" disabled={busy}>Запомнить меня</AuthCheckbox>
                    ) }

                    { isRegister && (
                        <div className={styles.consent}>
                            <AuthCheckbox ref={consentRef} name="personal_data_consent" disabled={busy}>
                                Я согласен на <Link className={styles.legalLink} href="/personal-data-consent" target="_blank" rel="noopener">обработку персональных данных</Link>
                            </AuthCheckbox>

                            <AuthCheckbox name="privacy_policy" disabled={busy}>
                                Я ознакомился с <Link className={styles.legalLink} href="/privacy-policy" target="_blank" rel="noopener">политикой конфиденциальности</Link>
                            </AuthCheckbox>
                        </div>
                    ) }

                    <AuthButton type="submit" loading={busy}>{ buttons[mode] }</AuthButton>
                </form>

                { isLogin ? (
                    <div className={styles.links}>
                        <Link className={styles.link} href="/forgot-password">Забыли пароль?</Link>
                        <Link className={styles.link} href="/register">Зарегистрироваться</Link>
                    </div>
                ) : (
                    <div className={styles.footer}>
                        { isCode && verificationEmail && <button className={`${styles.link} ${styles.requestLink}`} type="button" disabled={resendBusy || busy} aria-busy={resendBusy || undefined} onClick={resendCode}><span className={resendBusy ? styles.contentHidden : undefined} aria-hidden={resendBusy || undefined}>Отправить код повторно</span>{ resendBusy && <span className={styles.loading}><Spinner /></span> }</button> }
                        <p>{ isRegister ? "Уже есть аккаунт? " : isCode && verificationPurpose !== "login" ? "Указали не ту почту? " : "" }<Link className={styles.link} href={isCode ? codeBackUrl : "/login"}>{ isRegister ? "Войти" : isCode && verificationPurpose !== "login" ? "Обратно" : "Вернуться ко входу" }</Link></p>
                    </div>
                ) }
            </section>
        </main>
    );
};


export default AuthForm;
