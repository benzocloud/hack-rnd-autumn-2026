import assert from "node:assert/strict";
import test from "node:test";

import handleMockAuth from "../src/mock/auth.ts";
import { readMockState, fixture } from "../src/mock/state.ts";

const register = ( state ) => {
    const consent = fixture("PersonalDataConsentDocument");
    return handleMockAuth("/auth/register", { email: "unverified@example.com", display_name: "Demo", password: "example-password", personal_data_consent: { accepted: true, document_version: consent.document_version, document_sha256: consent.document_sha256 } }, state);
};

test("inert registration does not revoke a real verification handle", () => {
    const state = readMockState();
    const real = register(state);
    const inert = register(state);
    assert.notEqual(real.challenge_id, inert.challenge_id);
    assert.throws(() => handleMockAuth("/auth/verify-email", { challenge_id: inert.challenge_id, code: "012345" }, state), { code: "CODE_INVALID_OR_EXPIRED" });
    handleMockAuth("/auth/verify-email", { challenge_id: real.challenge_id, code: "012345" }, state);
    assert.equal(state.user, null);
    assert.equal(state.expiresAt, null);
});

test("verification updates an existing persisted session without creating a new session", () => {
    const original = readMockState();
    const challenge = register(original);
    original.user = original.registeredUsers.find(( user ) => user.email === "unverified@example.com");
    original.expiresAt = new Date(Date.now() + 600000).toISOString();
    const state = JSON.parse(JSON.stringify(original));
    handleMockAuth("/auth/verify-email", { challenge_id: challenge.challenge_id, code: "012345" }, state);
    assert.equal(state.user.email_verified, true);
    assert.equal(state.expiresAt, original.expiresAt);
    assert.equal(handleMockAuth("/me", null, state).email_verified, true);
});

test("expired challenge cannot verify an email", () => {
    const state = readMockState();
    const challenge = register(state);
    state.verificationChallenge.expires_at = new Date(Date.now() - 1).toISOString();
    assert.throws(() => handleMockAuth("/auth/verify-email", { challenge_id: challenge.challenge_id, code: "012345" }, state), { code: "CODE_INVALID_OR_EXPIRED" });
    assert.equal(state.registeredUsers.find(( user ) => user.email === "unverified@example.com").email_verified, false);
});
