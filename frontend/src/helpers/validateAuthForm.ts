import type { AuthMode } from "@/interfaces";

import type { FieldErrors } from "./fieldErrors";
import { isMockVerificationCode } from "./verificationCode";


const getAuthFieldErrors = ( mode: AuthMode, form: FormData ): FieldErrors => {
    const errors: FieldErrors = {};
    const value = ( name: string ) => String(form.get(name) || "");

    if (mode === "verify-code") {
        const name = form.has("code") ? "code" : "token";
        if (!isMockVerificationCode(value(name).trim())) {
            errors[name] = "Введите все шесть цифр кода подтверждения.";
        }
        return errors;
    }

    if (mode === "register") {
        const firstName = value("first_name").trim();
        const lastName = value("last_name").trim();
        if (!firstName) { errors.first_name = "Введите имя."; }
        if (!lastName) { errors.last_name = "Введите фамилию."; }
        if (firstName.length > 60 || lastName.length > 60 || `${firstName} ${lastName}`.length > 120) {
            const message = "Имя и фамилия вместе должны содержать не больше 120 символов, каждое — не больше 60.";
            if (firstName.length > 60 || `${firstName} ${lastName}`.length > 120) { errors.first_name = message; }
            if (lastName.length > 60 || `${firstName} ${lastName}`.length > 120) { errors.last_name = message; }
        }
    }

    if (mode !== "reset-password") {
        const email = value("email").trim();
        if (!email) {
            errors.email = "Введите почту.";
        } else if (email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
            errors.email = "Введите корректный адрес почты.";
        }
    }

    if (mode === "login" && !value("password").trim()) {
        errors.password = "Введите пароль.";
    }

    if (mode === "register" || mode === "reset-password") {
        const password = value("password");
        if (password.length < 12 || password.length > 128) {
            errors.password = "Пароль должен содержать от 12 до 128 символов.";
        }
        if (!value("confirm_password") || password !== value("confirm_password")) {
            errors.confirm_password = "Пароли не совпадают.";
        }
    }

    if (mode === "register") {
        if (form.get("personal_data_consent") !== "on") {
            errors.personal_data_consent = "Подтвердите согласие на обработку персональных данных.";
        }
        if (form.get("privacy_policy") !== "on") {
            errors.privacy_policy = "Подтвердите ознакомление с политикой конфиденциальности.";
        }
    }

    return errors;
};


const validateAuthForm = ( mode: AuthMode, form: FormData ): string | null => Object.values(getAuthFieldErrors(mode, form))[0] || null;


export { getAuthFieldErrors };
export default validateAuthForm;
