import type { AuthMode } from "@/interfaces";

import { isMockVerificationCode } from "./verificationCode";


const validatePassword = ( form: FormData ): string | null => {
    const password = String(form.get("password") || "");

    if (password.length < 12 || password.length > 128) {
        return "Пароль должен содержать от 12 до 128 символов.";
    }

    return password === form.get("confirm_password") ? null : "Пароли не совпадают.";
};


const validateAuthForm = ( mode: AuthMode, form: FormData, mockMode = process.env.developerMode === "mock" ): string | null => {
    if (mode === "verify-code") {
        const token = String(form.get("token") || "").trim();

        if (mockMode) {
            return isMockVerificationCode(token) ? null : "Введите все шесть цифр кода подтверждения.";
        }

        return token.length >= 16 && token.length <= 512 ? null : "Введите токен подтверждения из письма целиком.";
    }

    if (mode === "reset-password") {
        return validatePassword(form);
    }

    const email = String(form.get("email") || "").trim();

    if (mode === "register") {
        const firstName = String(form.get("first_name") || "").trim();
        const lastName = String(form.get("last_name") || "").trim();

        if (!firstName) {
            return "Введите имя.";
        }

        if (!lastName) {
            return "Введите фамилию.";
        }

        if (firstName.length > 60 || lastName.length > 60 || `${firstName} ${lastName}`.length > 120) {
            return "Имя и фамилия вместе должны содержать не больше 120 символов, каждое — не больше 60.";
        }
    }

    if (!email) {
        return "Введите почту.";
    }

    if (email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
        return "Введите корректный адрес почты.";
    }

    if (mode === "login" && !String(form.get("password") || "").trim()) {
        return "Введите пароль.";
    }

    if (mode === "register") {
        const passwordError = validatePassword(form);

        if (passwordError) {
            return passwordError;
        }

        if (form.get("personal_data_consent") !== "on") {
            return "Подтвердите согласие на обработку персональных данных.";
        }

        if (form.get("privacy_policy") !== "on") {
            return "Подтвердите ознакомление с политикой конфиденциальности.";
        }
    }

    return null;
};


export default validateAuthForm;
