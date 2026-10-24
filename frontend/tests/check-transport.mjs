import assert from "node:assert/strict";

import axios from "axios";


const calls = [];
const transport = async ( config ) => {
    calls.push(config);

    return { config, status: 200, statusText: "OK", headers: {}, data: config.url === "/auth/csrf" ? { csrf_token: "real-csrf", expires_at: "2099-01-01T00:00:00Z" } : { real: true } };
};


axios.defaults.adapter = transport;
global.window = {};

const { default: api } = await import("../src/service/api.ts");
const { authService, companiesService, accountService } = await import("../src/service/index.ts");

assert.equal(api.defaults.adapter, transport);
assert.deepEqual(await companiesService.listCompanies(), { real: true });
assert.deepEqual(await authService.login({ data: { email: "", password: "" } }), { real: true });
assert.deepEqual(calls.map(( item ) => item.url), [ "/companies", "/auth/csrf", "/auth/login" ]);
assert.equal(calls[2].headers.get("X-CSRF-Token"), "real-csrf");
assert.ok(calls.every(( item ) => item.withCredentials === true));

await assert.rejects(authService.confirmMockLogin({ data: { token: "000000" } }));
await assert.rejects(authService.confirmMockPasswordReset({ data: { token: "000000" } }));
assert.equal(calls.length, 3);

assert.deepEqual(await accountService.checkSession(), { real: true });
assert.equal(calls.at(-1).url, "/me");
assert.equal(calls.at(-1).withCredentials, true);

await authService.getPersonalDataConsentDocument();
assert.equal(calls.at(-1).url, "/auth/personal-data-consent");
const verification = { challenge_id: "550e8400-e29b-41d4-a716-446655440000", code: "012345" };
await authService.verifyEmail({ data: verification });
assert.equal(calls.at(-1).url, "/auth/verify-email");
assert.deepEqual(JSON.parse(calls.at(-1).data), verification);
assert.equal(calls.at(-1).headers.get("X-CSRF-Token"), "real-csrf");
assert.equal(calls.at(-1).withCredentials, true);
await authService.resendVerification({ data: { email: "test@example.com" } });
assert.equal(calls.at(-1).url, "/auth/resend-verification");
assert.deepEqual(JSON.parse(calls.at(-1).data), { email: "test@example.com" });
