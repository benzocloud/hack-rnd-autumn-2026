import assert from "node:assert/strict";
import test from "node:test";

import { getFieldErrors, validateTextFields } from "../src/helpers/fieldErrors.ts";
import { getAuthFieldErrors } from "../src/helpers/validateAuthForm.ts";
import { isKnownRoute } from "../src/helpers/auth.ts";


test("all invalid auth fields are reported together without changing submitted values", () => {
    const form = new FormData();
    form.set("email", "not-an-email");
    form.set("password", " ");
    assert.deepEqual(Object.keys(getAuthFieldErrors("login", form)), [ "email", "password" ]);
    assert.equal(form.get("password"), " ");
    assert.deepEqual(Object.keys(getAuthFieldErrors("register", new FormData())), [ "first_name", "last_name", "email", "password", "confirm_password", "personal_data_consent", "privacy_policy" ]);

    form.set("password", "valid-new-password");
    form.set("confirm_password", "different-password");
    assert.deepEqual(Object.keys(getAuthFieldErrors("reset-password", form)), [ "confirm_password" ]);
});


test("financial fields collect independent failures and preserve signed dated balances", () => {
    const fields = [
        { name: "amount", label: "Сумма", value: "wrong", required: true, money: true },
        { name: "counterparty_name", label: "Контрагент", value: "  ", required: true },
        { name: "scheduled_date", label: "Дата", value: "2026-02-30", type: "date" },
        { name: "inn", label: "ИНН", value: "123" },
    ];
    assert.deepEqual(Object.keys(validateTextFields(fields)), fields.map(( field ) => field.name));
    assert.deepEqual(validateTextFields([{ name: "amount", label: "Остаток", value: "-123,45", money: true, allowNegative: true }, { name: "inn", label: "ИНН", value: "" }]), {});
    assert.ok(validateTextFields([{ name: "email", label: "Почта", value: "bad", type: "email" }]).email);
});


test("API validation errors map to every affected form field; global errors do not mark unrelated fields", () => {
    const cause = { isAxiosError: true, response: { data: { error: { field_errors: [ { field: "data.display_name", message: "Слишком длинное имя" }, { field: "new_password", message: "Пароль слишком короткий" }, { field: "balance.amount_minor", message: "Неверная сумма" } ] } } } };
    assert.deepEqual(getFieldErrors(cause), { first_name: "Слишком длинное имя", last_name: "Слишком длинное имя", password: "Пароль слишком короткий", amount: "Неверная сумма" });
    assert.deepEqual(getFieldErrors(new Error("Нет соединения")), {});
    assert.deepEqual(getFieldErrors(Object.assign(new Error("Неверная дата"), { field: "date" })), { date: "Неверная дата" });
});


test("404 is public without bypassing known protected application routes", () => {
    for (const path of [ "/", "/workspace", "/account", "/login", "/companies/company-id/settings", "/companies/company-id/forecasts/forecast-id" ]) {
        assert.equal(isKnownRoute(path), true, path);
    }
    for (const path of [ "/missing", "/companies/company-id/missing" ]) {
        assert.equal(isKnownRoute(path), false, path);
    }
});
