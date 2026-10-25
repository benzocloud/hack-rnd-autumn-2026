import assert from "node:assert/strict";
import test from "node:test";

import { rememberCompanyCreateKey } from "../src/helpers/companyCreate.ts";


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
