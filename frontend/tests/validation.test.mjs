import assert from "node:assert/strict";
import test from "node:test";

import validateAuthForm from "../src/helpers/validateAuthForm.ts";


const form = ( values = {} ) => {
    const data = new FormData();

    Object.entries(values).forEach(( [ key, value ] ) => data.set(key, value));

    return data;
};


test("login has no client validation, including empty and malformed values", () => {
    assert.equal(validateAuthForm("login", form()), null);
    assert.equal(validateAuthForm("login", form({ email: "invalid", password: "1" })), null);
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

    for (const token of [ "", "0", "000", "00000", "abcd" ]) {
        assert.ok(validateAuthForm("verify-code", form({ token }), true));
    }

    assert.equal(validateAuthForm("verify-code", form({ token: "4837" }), true), null);
});


test("new password is validated in code without requiring email or registration consents", () => {
    assert.equal(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "new-demo-password" })), null);
    assert.match(validateAuthForm("reset-password", form({ password: "short", confirm_password: "short" })), /12/);
    assert.match(validateAuthForm("reset-password", form({ password: "new-demo-password", confirm_password: "different-password" })), /совпадают/);
});


test("real verification accepts the full backend token, never a locally approved short code", () => {
    assert.ok(validateAuthForm("verify-code", form({ token: "1234" }), false));
    assert.equal(validateAuthForm("verify-code", form({ token: "a-valid-backend-token-123456" }), false), null);
});
