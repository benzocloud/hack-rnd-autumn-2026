import assert from "node:assert/strict";
import test from "node:test";

import validateAuthForm from "../src/helpers/validateAuthForm.ts";


const form = ( values = {} ) => {
    const data = new FormData();

    Object.entries(values).forEach(( [ key, value ] ) => data.set(key, value));

    return data;
};


test("login rejects empty or malformed email and blank passwords", () => {
    assert.match(validateAuthForm("login", form()), /почту/);
    assert.match(validateAuthForm("login", form({ email: "   ", password: "password" })), /почту/);
    assert.match(validateAuthForm("login", form({ email: "not-an-email", password: " " })), /адрес почты/);
    assert.match(validateAuthForm("login", form({ email: "test@example.com", password: "" })), /пароль/);
    assert.match(validateAuthForm("login", form({ email: "test@example.com", password: " \t\n " })), /пароль/);
});


test("login accepts filled credentials without applying new-password rules", () => {
    const values = { email: " test@example.com ", password: "1" };
    const data = form(values);

    assert.equal(validateAuthForm("login", data), null);
    assert.equal(data.get("email"), values.email);
    assert.equal(data.get("password"), values.password);
    assert.equal(validateAuthForm("login", form({ email: "test@example.com", password: "  existing-password  " })), null);
});


test("registration checks each field and both independent confirmations in code", () => {
    const values = { first_name: "Иван", last_name: "Иванов", email: "test@example.com", password: "example-password", confirm_password: "example-password", personal_data_consent: "on", privacy_policy: "on" };

    assert.equal(validateAuthForm("register", form(values)), null);

    for (const field of Object.keys(values)) {
        assert.ok(validateAuthForm("register", form({ ...values, [field]: "" })), field);
    }

    assert.match(validateAuthForm("register", form({ ...values, email: "invalid" })), /почты/);
    assert.match(validateAuthForm("register", form({ ...values, password: "short" })), /12/);
    assert.match(validateAuthForm("register", form({ ...values, confirm_password: "different-password" })), /совпадают/);
});


test("recovery and code errors are returned for snackbar display", () => {
    assert.ok(validateAuthForm("forgot-password", form()));
    assert.ok(validateAuthForm("forgot-password", form({ email: "invalid" })));
    assert.equal(validateAuthForm("forgot-password", form({ email: "test@example.com" })), null);

    for (const token of [ "", "0", "0000", "00000", "0000000", "abcdef" ]) {
        assert.ok(validateAuthForm("verify-code", form({ token })));
    }

    assert.equal(validateAuthForm("verify-code", form({ token: "012345" })), null);
});


test("new password is validated in code without requiring email or registration consents", () => {
    assert.equal(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "new-demo-password" })), null);
    assert.match(validateAuthForm("reset-password", form({ password: "short", confirm_password: "short" })), /12/);
    assert.match(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "different-password" })), /совпадают/);
});


test("email verification requires exactly six ASCII digits, preserving leading zeros", () => {
    for (const code of [ "1234", "1234567", "abcdef", "１２３４５６", "a-valid-backend-token-123456" ]) {
        assert.ok(validateAuthForm("verify-code", form({ code })));
    }
    assert.equal(validateAuthForm("verify-code", form({ code: "012345" })), null);
});
