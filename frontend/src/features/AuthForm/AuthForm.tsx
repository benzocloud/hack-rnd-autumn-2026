"use client";

import axios from "axios";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { useDispatch, useSelector } from "react-redux";

import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar, showSuccessSnackbar } from "@/helpers/snackbar";
import validateAuthForm from "@/helpers/validateAuthForm";
import type { AuthMode, IPersonalDataConsentDocument, VerificationPurpose } from "@/interfaces";
import { authService } from "@/service";
import { clearAuthUser, setAuthUser, setVerificationChallenge, setVerificationEmail } from "@/store/authSlice";
import { setSelectedCompany } from "@/store/companySlice";
import type { AppDispatch, RootState } from "@/store/store";
import { AuthButton, AuthCheckbox, AuthInput, CodeInput, Spinner } from "@/widgets";

import styles from "./style.module.scss";


interface IProps {
    mode: AuthMode;
    verificationPurpose?: VerificationPurpose;
    resetToken?: string;
}


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


const AuthForm = ( { mode, verificationPurpose = "register", resetToken = "" }: IProps ) => {
    const [ busy, setBusy ] = useState(false);
    const [ resendBusy, setResendBusy ] = useState(false);
    const [ consentDocument, setConsentDocument ] = useState<IPersonalDataConsentDocument | null>(null);
    const [ consentFailed, setConsentFailed ] = useState(false);
    const { user, verificationEmail, verificationChallenge } = useSelector(( state: RootState ) => state.auth);

    const router = useRouter();
    const dispatch = useDispatch<AppDispatch>();
    const emailRef = useRef<HTMLInputElement>(null);
    const rememberRef = useRef<HTMLInputElement>(null);
    const consentRef = useRef<HTMLInputElement>(null);
    const requestRef = useRef<AbortController | null>(null);
    const resendRef = useRef<AbortController | null>(null);
    const consentRequestRef = useRef<AbortController | null>(null);
    const isRegister = mode === "register";
    const isLogin = mode === "login";
    const isCode = mode === "verify-code";
    const isReset = mode === "reset-password";
    const codeTitles: Record<VerificationPurpose, string> = { register: "Подтверждение регистрации", login: "Подтверждение входа", reset: "Подтверждение сброса пароля" };
    const codeBackUrl = verificationPurpose === "register" ? "/register" : verificationPurpose === "reset" ? "/forgot-password" : "/login";


    const loadConsentDocument = useCallback(() => {
        consentRequestRef.current?.abort();
        const controller = new AbortController();
        consentRequestRef.current = controller;
        return authService.getPersonalDataConsentDocument(controller.signal).then(( document ) => {
            if (!controller.signal.aborted) {
                setConsentDocument(document);
            }
        }, () => {
            if (!controller.signal.aborted) {
                setConsentFailed(true);
            }
        });
    }, []);


    const reloadConsentDocument = () => {
        setConsentDocument(null);
        setConsentFailed(false);
        if (consentRef.current) {
            consentRef.current.checked = false;
        }
        return loadConsentDocument();
    };


    useEffect(() => {
        if (isRegister) {
            void loadConsentDocument();
        }
        return () => consentRequestRef.current?.abort();
    }, [ isRegister, loadConsentDocument ]);


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

        if (isRegister && !consentDocument) {
            showErrorSnackbar("Дождитесь загрузки актуального текста согласия.");
            return;
        }

        if (isCode && verificationPurpose === "register" && (!verificationChallenge || Date.parse(verificationChallenge.expires_at) <= Date.now())) {
            showErrorSnackbar("Запросите новый код подтверждения.");
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
                    dispatch(setVerificationChallenge(null));
                    showSuccessSnackbar("Для подтверждения входа введите любые шесть цифр.");
                    router.push("/verify-code?purpose=login");
                } else {
                    dispatch(setAuthUser(response.user));
                    dispatch(setVerificationEmail(response.user.email));
                    dispatch(setVerificationChallenge(null));
                    router.replace(response.user.email_verified ? "/" : "/verify-code");
                }
            } else if (isRegister && consentDocument) {
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
                dispatch(setVerificationChallenge(response));
                showSuccessSnackbar(response.message);
                router.push("/verify-code");
            } else if (isCode) {
                const token = String(form.get("code") || form.get("token") || "").trim();

                if (verificationPurpose === "login") {
                    const response = await authService.confirmMockLogin({ data: { token } }, controller.signal);

                    dispatch(setAuthUser(response.user));
                    showSuccessSnackbar("Вход подтверждён.");
                    router.replace(response.user.email_verified ? "/" : "/verify-code");
                } else if (verificationPurpose === "reset") {
                    const response = await authService.confirmMockPasswordReset({ data: { token } }, controller.signal);

                    showSuccessSnackbar(response.message);
                    router.replace(`/reset-password?token=${encodeURIComponent(token)}`);
                } else if (verificationChallenge) {
                    const response = await authService.verifyEmail({ data: { challenge_id: verificationChallenge.challenge_id, code: token } }, controller.signal);

                    dispatch(clearAuthUser());
                    dispatch(setVerificationChallenge(null));
                    showSuccessSnackbar(response.message);
                    router.replace("/login");
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
                await reloadConsentDocument();
            }
        } finally {
            requestRef.current = null;
            setBusy(false);
        }
    };


    const resendCode = async () => {
        const email = verificationEmail || user?.email || emailRef.current?.value.trim();
        if (resendRef.current || !email) {
            return;
        }

        const controller = new AbortController();

        resendRef.current = controller;
        setResendBusy(true);

        try {
            if (verificationPurpose === "reset") {
                const response = await authService.forgotPassword({ data: { email } }, controller.signal);
                showSuccessSnackbar(response.message);
            } else {
                const response = await authService.resendVerification({ data: { email } }, controller.signal);
                dispatch(setVerificationChallenge(response));
                showSuccessSnackbar(response.message);
            }

            dispatch(setVerificationEmail(email));
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
                        { isCode && !verificationEmail && !user?.email && <AuthInput ref={emailRef} name="email" label="Почта для повторного кода" type="email" autoComplete="email" /> }
                        { isCode && <CodeInput name={verificationPurpose === "register" ? "code" : "token"} /> }
                    </fieldset>

                    { isLogin && (
                        <AuthCheckbox ref={rememberRef} name="remember" disabled={busy}>Запомнить меня</AuthCheckbox>
                    ) }

                    { isRegister && (
                        <div className={styles.consent}>
                            { consentFailed && <button type="button" onClick={() => void reloadConsentDocument()}>Повторить загрузку</button> }
                            <AuthCheckbox ref={consentRef} name="personal_data_consent" disabled={busy || !consentDocument}>
                                Я согласен на обработку персональных данных по тексту выше
                            </AuthCheckbox>

                            <AuthCheckbox name="privacy_policy" disabled={busy}>
                                Я ознакомился с <Link className={styles.legalLink} href="/privacy-policy" target="_blank" rel="noopener">политикой конфиденциальности</Link>
                            </AuthCheckbox>
                        </div>
                    ) }

                    <AuthButton type="submit" loading={busy} disabled={isRegister && !consentDocument}>{ buttons[mode] }</AuthButton>
                </form>

                { isLogin ? (
                    <div className={styles.links}>
                        <Link className={styles.link} href="/forgot-password">Забыли пароль?</Link>
                        <Link className={styles.link} href="/register">Зарегистрироваться</Link>
                    </div>
                ) : (
                    <div className={styles.footer}>
                        { isCode && user && <Link className={styles.link} href="/account">Аккаунт и выход</Link> }
                        { isCode && <button className={`${styles.link} ${styles.requestLink}`} type="button" disabled={resendBusy || busy} aria-busy={resendBusy || undefined} onClick={resendCode}><span className={resendBusy ? styles.contentHidden : undefined} aria-hidden={resendBusy || undefined}>Отправить код повторно</span>{ resendBusy && <span className={styles.loading}><Spinner /></span> }</button> }
                        <p>{ isRegister ? "Уже есть аккаунт? " : isCode && verificationPurpose !== "login" ? "Указали не ту почту? " : "" }<Link className={styles.link} href={isCode ? codeBackUrl : "/login"}>{ isRegister ? "Войти" : isCode && verificationPurpose !== "login" ? "Обратно" : "Вернуться ко входу" }</Link></p>
                    </div>
                ) }
            </section>
        </main>
    );
};


export default AuthForm;
