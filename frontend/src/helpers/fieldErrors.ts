import axios from "axios";

import { dateField, parseMoney, textField } from "./finance";


type FieldErrors = Record<string, string>;

interface ITextFieldRule {
    name: string;
    label: string;
    value: string;
    required?: boolean;
    maximum?: number;
    type?: string;
    money?: boolean;
    allowNegative?: boolean;
}


const validateTextFields = ( fields: ITextFieldRule[] ): FieldErrors => {
    const errors: FieldErrors = {};

    for (const field of fields) {
        const form = new FormData();
        form.set(field.name, field.value);

        try {
            const value = textField(form, field.name, field.label, field.maximum ?? 200, !field.required);

            if (!value) { continue; }
            if (field.type === "date") { dateField(form, field.name, field.label); }
            if (field.money) { parseMoney(value, field.allowNegative); }
            if (field.type === "email" && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
                errors[field.name] = "Укажите корректный адрес почты.";
            }
            if (field.name === "inn" && !/^([0-9]{10}|[0-9]{12})$/.test(value)) {
                errors[field.name] = "ИНН должен содержать 10 или 12 цифр.";
            }
        } catch (cause) {
            errors[field.name] = cause instanceof Error ? cause.message : "Проверьте значение поля.";
        }
    }

    return errors;
};


const getFieldErrors = ( cause: unknown ): FieldErrors => {
    const errors: FieldErrors = {};
    const detail = axios.isAxiosError(cause) ? cause.response?.data?.error : cause;
    const items = detail && typeof detail === "object" && "field_errors" in detail && Array.isArray(detail.field_errors) ? detail.field_errors : [ detail ];

    for (const item of items) {
        if (!item || typeof item !== "object" || !("field" in item) || typeof item.field !== "string" || !item.field) { continue; }
        const field = item.field.replace(/\[(\w+)\]/g, ".$1").split(".").at(-1) || "";
        const names = field === "display_name" ? [ "first_name", "last_name" ] : [ ({ new_password: "password", amount_minor: "amount", balance: "amount", cutoff_date: "cutoff_date" } as Record<string, string>)[field] || field ];
        const message = "message" in item && typeof item.message === "string" && item.message ? item.message : "Проверьте значение поля.";
        for (const name of names) { errors[name] = message; }
    }

    return errors;
};


export { getFieldErrors, validateTextFields };
export type { FieldErrors, ITextFieldRule };
