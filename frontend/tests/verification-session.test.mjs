import assert from "node:assert/strict";
import test from "node:test";

import { readVerificationSessionFrom, storageName, writeVerificationSessionTo } from "../src/helpers/verificationSession.ts";


const memory = () => {
    const values = new Map();

    return {
        getItem: ( key ) => values.has(key) ? values.get(key) : null,
        setItem: ( key, value ) => values.set(key, value),
        removeItem: ( key ) => values.delete(key),
    };
};


test("verification session survives a new page with the same storage", () => {
    const store = memory();
    const challenge = {
        challenge_id: "11111111-1111-4111-8111-111111111111",
        expires_at: new Date(Date.now() + 60_000).toISOString(),
        message: "Код отправлен",
    };

    writeVerificationSessionTo(store, { email: "financier@saldo.test", challenge });

    const restored = readVerificationSessionFrom(store);

    assert.equal(restored.email, "financier@saldo.test");
    assert.deepEqual(restored.challenge, challenge);
    assert.ok(store.getItem(storageName));
});


test("expired or broken verification sessions are dropped", () => {
    const store = memory();

    writeVerificationSessionTo(store, {
        email: "owner@saldo.test",
        challenge: {
            challenge_id: "11111111-1111-4111-8111-111111111111",
            expires_at: new Date(Date.now() - 1).toISOString(),
            message: "Код отправлен",
        },
    });

    assert.deepEqual(readVerificationSessionFrom(store), { email: "owner@saldo.test", challenge: null });

    store.setItem(storageName, "{");
    assert.deepEqual(readVerificationSessionFrom(store), { email: "", challenge: null });
});


test("clearing verification session removes storage", () => {
    const store = memory();

    writeVerificationSessionTo(store, {
        email: "a@b.test",
        challenge: {
            challenge_id: "11111111-1111-4111-8111-111111111111",
            expires_at: new Date(Date.now() + 60_000).toISOString(),
            message: "ok",
        },
    });
    writeVerificationSessionTo(store, { email: "", challenge: null });

    assert.equal(store.getItem(storageName), null);
});
