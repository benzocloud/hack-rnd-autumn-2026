import assert from "node:assert/strict";
import test from "node:test";

import validateAuthForm from "../src/helpers/validateAuthForm.ts";


const form = ( values = {} ) => {
    const data = new FormData();

    Object.entries(values).forEach(( [ key, value ] ) => data.set(key, value));

    return data;
};


test("login rejects empty or malformed email and blank passwords in every mode", () => {
    for (const mockMode of [ true, false ]) {
        assert.match(validateAuthForm("login", form(), mockMode), /почту/);
        assert.match(validateAuthForm("login", form({ email: "   ", password: "password" }), mockMode), /почту/);
        assert.match(validateAuthForm("login", form({ email: "not-an-email", password: " " }), mockMode), /адрес почты/);
        assert.match(validateAuthForm("login", form({ email: "test@example.com", password: "" }), mockMode), /пароль/);
        assert.match(validateAuthForm("login", form({ email: "test@example.com", password: " \t\n " }), mockMode), /пароль/);
    }
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
        assert.ok(validateAuthForm("verify-code", form({ token }), true));
    }

    assert.equal(validateAuthForm("verify-code", form({ token: "012345" }), true), null);
});


test("new password is validated in code without requiring email or registration consents", () => {
    assert.equal(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "new-demo-password" })), null);
    assert.match(validateAuthForm("reset-password", form({ password: "short", confirm_password: "short" })), /12/);
    assert.match(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "different-password" })), /совпадают/);
});


test("real verification accepts the full backend token, never a locally approved short code", () => {
    assert.ok(validateAuthForm("verify-code", form({ token: "1234" }), false));
    assert.ok(validateAuthForm("verify-code", form({ token: "012345" }), false));
    assert.equal(validateAuthForm("verify-code", form({ token: "a-valid-backend-token-123456" }), false), null);
});
