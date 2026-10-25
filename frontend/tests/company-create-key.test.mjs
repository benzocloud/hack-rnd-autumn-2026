import assert from "node:assert/strict";
import test from "node:test";

import { clearCompanyCreateKey, companyCreateKeyFrom, rememberCompanyCreateKey } from "../src/helpers/companyCreate.ts";


const memory = () => {
    const values = new Map();

    return {
        getItem: ( key ) => values.has(key) ? values.get(key) : null,
        setItem: ( key, value ) => values.set(key, value),
    };
};


test("a company creation retry keeps the key for the same form, including after a new page", () => {
    const first = memory();
    const created = rememberCompanyCreateKey(first, "name");
    const again = rememberCompanyCreateKey(first, "name");
    const restored = memory();

    restored.setItem("saldo-company-create:name", created);

    assert.equal(again, created);
    assert.equal(rememberCompanyCreateKey(restored, "name"), created);
    assert.notEqual(rememberCompanyCreateKey(first, "other"), created);
});


test("a blocked browser keeps one key for the form until creation is confirmed", () => {
    const blocked = {
        getItem: () => { throw new Error("blocked"); },
        setItem: () => { throw new Error("blocked"); },
    };
    const first = companyCreateKeyFrom(blocked, "offline");

    assert.equal(companyCreateKeyFrom(blocked, "offline"), first);
    assert.notEqual(companyCreateKeyFrom(blocked, "offline-other"), first);
    clearCompanyCreateKey("offline");
    assert.notEqual(companyCreateKeyFrom(blocked, "offline"), first);
});
