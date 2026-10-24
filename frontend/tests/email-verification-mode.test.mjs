import assert from "node:assert/strict";
import test from "node:test";

import { getEmailVerificationMode } from "../src/helpers/emailVerificationMode.ts";


test("email mock label requires an explicit server response, never a browser flag", () => {
    assert.equal(getEmailVerificationMode({ email_verification_mode: "demo" }), "demo");
    assert.equal(getEmailVerificationMode({ email_verification_mode: "strict" }), "strict");
    for (const response of [
        undefined,
        null,
        {},
        { email_verification_mode: "DEMO" },
        { email_verification_mode: true },
        { email_verification_mode: "unknown" },
        { AUTH_DEMO_EMAIL_VERIFICATION: true },
        { developerMode: "mock" },
    ]) {
        assert.equal(getEmailVerificationMode(response), null);
    }
});
