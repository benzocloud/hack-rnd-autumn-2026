import type { IEmailRequest, ILoginRequest, IRegisterRequest, ITokenRequest, IUser } from "@/interfaces";

import { expiresAt, mockError, mockId } from "./helpers";
import { fixture, type IMockState } from "./state";


const handleMockAuth = ( path: string, data: unknown, state: IMockState ): unknown => {
    if (path === "/auth/csrf") {
        return { ...fixture<object>("CsrfToken"), expires_at: expiresAt() };
    }

    if (path === "/auth/personal-data-consent") {
        return fixture("PersonalDataConsentDocument");
    }

    if (path === "/auth/register") {
        const request = data as IRegisterRequest;
        const document = fixture<{ document_version: string; document_sha256: string }>("PersonalDataConsentDocument");

        if (!request.personal_data_consent?.accepted) {
            throw mockError(400, "Подтвердите согласие на обработку персональных данных.");
        }

        if (request.personal_data_consent.document_version !== document.document_version || request.personal_data_consent.document_sha256 !== document.document_sha256) {
            throw mockError(409, "Текст согласия изменился. Обновите страницу и подтвердите его снова.", "CONSENT_DOCUMENT_CHANGED");
        }

        const existing = state.registeredUsers.find(( user ) => user.email === request.email);
        const user: IUser = { id: existing?.id || mockId("usr"), email: request.email, display_name: request.display_name, email_verified: false };

        state.registeredUsers = [ ...state.registeredUsers.filter(( item ) => item.id !== user.id), user ];
        state.pendingEmail = request.email;

        return { message: "Регистрация завершена. Для подтверждения введите любые четыре цифры." };
    }

    if (path === "/auth/login") {
        const request = data as ILoginRequest;
        const email = request.email || "owner@example.com";
        const existing = state.registeredUsers.find(( user ) => user.email === email);
        const user: IUser = existing || { ...fixture<IUser>("User"), id: mockId("usr"), email, display_name: email.split("@")[0] };

        state.pendingLoginUser = user;
        state.user = null;
        state.expiresAt = null;
        state.registeredUsers = [ ...state.registeredUsers.filter(( item ) => item.id !== user.id), user ];

        return { user, expires_at: expiresAt() };
    }

    if (path === "/auth/mock/confirm-login") {
        if (!/^\d{4}$/.test((data as ITokenRequest).token)) {
            throw mockError(400, "Введите четырёхзначный код. В режиме mock подойдут любые цифры.", "INVALID_TOKEN");
        }

        if (!state.pendingLoginUser) {
            throw mockError(401, "Сначала введите данные для входа.", "UNAUTHORIZED");
        }

        state.user = state.pendingLoginUser;
        state.pendingLoginUser = null;
        state.expiresAt = expiresAt();

        return { user: state.user, expires_at: state.expiresAt };
    }

    if (path === "/auth/mock/confirm-password-reset") {
        if (!/^\d{4}$/.test((data as ITokenRequest).token) || !state.resetEmail) {
            throw mockError(400, "Запросите восстановление пароля и введите любые четыре цифры.", "INVALID_TOKEN");
        }

        state.passwordResetVerified = true;

        return { message: "Код подтверждён. Задайте новый пароль." };
    }

    if (path === "/auth/verify-email") {
        if (!/^\d{4}$/.test((data as ITokenRequest).token)) {
            throw mockError(400, "Введите четырёхзначный код. В режиме mock подойдут любые цифры.", "INVALID_TOKEN");
        }

        const user = state.registeredUsers.find(( item ) => item.email === state.pendingEmail);

        if (!user) {
            throw mockError(400, "Сначала зарегистрируйте аккаунт.", "INVALID_TOKEN");
        }

        user.email_verified = true;
        state.user = user;
        state.expiresAt = expiresAt();
        state.pendingEmail = "";
        state.pendingLoginUser = null;

        return { message: "Почта подтверждена. Вы вошли в аккаунт." };
    }

    if (path === "/auth/resend-verification") {
        state.pendingEmail = (data as IEmailRequest).email;

        return { message: "Введите любые четыре цифры. Письмо в режиме mock не отправляется." };
    }

    if (path === "/auth/forgot-password") {
        state.resetEmail = (data as IEmailRequest).email;
        state.passwordResetVerified = false;

        return { message: "Запрос принят. Для подтверждения введите любые четыре цифры." };
    }

    if (path === "/auth/reset-password") {
        if (!/^\d{4}$/.test((data as ITokenRequest).token) || !state.passwordResetVerified) {
            throw mockError(400, "Сначала подтвердите сброс пароля четырёхзначным кодом.", "INVALID_TOKEN");
        }

        state.user = null;
        state.expiresAt = null;

        state.pendingLoginUser = null;
        state.passwordResetVerified = false;
        state.resetEmail = "";

        return { message: "Пароль сброшен. В режиме mock можно войти с любым паролем." };
    }

    if (path === "/auth/logout") {
        state.pendingLoginUser = null;
        state.user = null;
        state.expiresAt = null;

        return undefined;
    }

    if (!state.user || !state.expiresAt || Date.parse(state.expiresAt) <= Date.now()) {
        throw mockError(401, "Войдите в аккаунт.", "UNAUTHORIZED");
    }

    return state.user;
};


export default handleMockAuth;
