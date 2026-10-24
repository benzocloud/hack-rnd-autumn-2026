import type { IAcknowledgement, ICsrfToken, IForgotPasswordParams, ILoginParams, IPersonalDataConsentDocument, IRegisterParams, IResendVerificationParams, IResetPasswordParams, ISession, IVerifyEmailParams } from "@/interfaces";

import api from "../api";


const getCsrfToken = async ( signal?: AbortSignal ): Promise<ICsrfToken> => {
    const response = await api.request<ICsrfToken>({
        method: "GET",
        url: "/auth/csrf",
        signal,
    });


    return response.data;
};


const getPersonalDataConsentDocument = async ( signal?: AbortSignal ): Promise<IPersonalDataConsentDocument> => {
    const response = await api.get<IPersonalDataConsentDocument>("/auth/personal-data-consent", { signal });

    return response.data;
};


const register = async ( request: IRegisterParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    const response = await api.request<IAcknowledgement>({
        method: "POST",
        url: "/auth/register",
        signal,
        data: request.data,
    });


    return response.data;
};


const login = async ( request: ILoginParams, signal?: AbortSignal ): Promise<ISession> => {
    const response = await api.request<ISession>({
        method: "POST",
        url: "/auth/login",
        signal,
        data: request.data,
    });


    return response.data;
};


const verifyEmail = async ( request: IVerifyEmailParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    const response = await api.request<IAcknowledgement>({
        method: "POST",
        url: "/auth/verify-email",
        signal,
        data: request.data,
    });


    return response.data;
};


const resendVerification = async ( request: IResendVerificationParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    const response = await api.request<IAcknowledgement>({
        method: "POST",
        url: "/auth/resend-verification",
        signal,
        data: request.data,
    });


    return response.data;
};


const forgotPassword = async ( request: IForgotPasswordParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    const response = await api.request<IAcknowledgement>({
        method: "POST",
        url: "/auth/forgot-password",
        signal,
        data: request.data,
    });


    return response.data;
};


const resetPassword = async ( request: IResetPasswordParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    const response = await api.request<IAcknowledgement>({
        method: "POST",
        url: "/auth/reset-password",
        signal,
        data: request.data,
    });


    return response.data;
};


const confirmMockLogin = async ( request: IVerifyEmailParams, signal?: AbortSignal ): Promise<ISession> => {
    if (process.env.developerMode !== "mock") {
        throw new Error("Подтверждение входа кодом пока недоступно. Вернитесь ко входу.");
    }

    const response = await api.post<ISession>("/auth/mock/confirm-login", request.data, { signal });

    return response.data;
};


const confirmMockPasswordReset = async ( request: IVerifyEmailParams, signal?: AbortSignal ): Promise<IAcknowledgement> => {
    if (process.env.developerMode !== "mock") {
        throw new Error("Откройте ссылку восстановления из письма.");
    }

    const response = await api.post<IAcknowledgement>("/auth/mock/confirm-password-reset", request.data, { signal });

    return response.data;
};


const logout = async ( signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "POST",
        url: "/auth/logout",
        signal,
    });


    return response.data;
};


const authService = { getCsrfToken, getPersonalDataConsentDocument, register, login, verifyEmail, resendVerification, forgotPassword, resetPassword, confirmMockLogin, confirmMockPasswordReset, logout };


export default authService;
