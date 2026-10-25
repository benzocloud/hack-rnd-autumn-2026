import assert from "node:assert/strict";
import test from "node:test";
import axios from "axios";

// Adapter regression only: these replies are fixtures, never real backend acceptance.
const calls = [];
axios.defaults.adapter = async (config) => {
    calls.push(config);
    return { config, status: 200, statusText: "OK", headers: {}, data: config.url === "/auth/csrf" ? { csrf_token: "fixture-csrf", expires_at: "2099-01-01T00:00:00Z" } : { id: "saved", version: "9007199254740993", status: "active", remaining_amount: { amount_minor: "9007199254740993", currency: "RUB" }, items: [], next_cursor: null } };
};
global.window = {};
const { default: payments } = await import("../src/service/payments_service/payments_service.ts");
const body = () => JSON.parse(calls.at(-1).data);

test("payment transport preserves scoped IDs, exact money, CAS and cancelled history filters", async () => {
    const signal = new AbortController().signal;
    await payments.listPayments({ company_id: "company/foreign", status: "cancelled", from: "2026-10-01", to: "2026-10-31", cursor: "page2", limit: 20 }, signal);
    assert.equal(calls.at(-1).url, "/companies/company%2Fforeign/payments");
    assert.equal(calls.at(-1).params.status, "cancelled");
    assert.equal(calls.at(-1).params.cursor, "page2");
    assert.equal(calls.at(-1).signal, signal);
    const data = { amount: { amount_minor: "9007199254740993", currency: "RUB" }, scheduled_date: "2026-10-30", version: "9007199254740993", reconciliation_group_id: "explicit-id" };
    await payments.updatePayment({ company_id: "company", payment_id: "payment", data }, signal);
    assert.equal(calls.at(-1).method, "patch");
    assert.deepEqual(body(), data);
    assert.equal(calls.at(-1).headers.get("X-CSRF-Token"), "fixture-csrf");
    await payments.cancelPayment({ company_id: "company", payment_id: "payment", version: data.version }, signal);
    assert.equal(calls.at(-1).method, "delete");
    assert.deepEqual(calls.at(-1).params, { version: data.version });
    await payments.matchPayment({ company_id: "company", payment_id: "payment", idempotency_key: "stable-key", data: { transaction_id: "exact-transaction-id", amount: data.amount, payment_version: data.version, transaction_version: "12" } }, signal);
    assert.equal(calls.at(-1).headers.get("Idempotency-Key"), "stable-key");
    assert.equal(body().transaction_id, "exact-transaction-id");
    await payments.removePaymentMatch({ company_id: "company", payment_id: "payment", match_id: "match", payment_version: "13" }, signal);
    assert.equal(calls.at(-1).url, "/companies/company/payments/payment/matches/match");
    assert.deepEqual(calls.at(-1).params, { payment_version: "13" });
});

test("reconciliation sends exactly the explicit additional or complete replacement branch", async () => {
    const window = { cutoff_date: "2026-10-25", horizon_start: "2026-10-26", horizon_end: "2026-11-24" };
    for (const branch of [ { kind: "additional", additional: { confirmed: true } }, { kind: "replacement", replacement: { complete_for_horizon: true, historical_operations: [{ transaction_id: "selected-id", version: "9007199254740993" }] } } ]) {
        const data = { ...window, ...branch };
        await payments.createReconciliationGroup({ company_id: "company", idempotency_key: "same-key-on-retry", data });
        assert.deepEqual(body(), data);
        assert.equal(calls.at(-1).headers.get("Idempotency-Key"), "same-key-on-retry");
        assert.equal(calls.at(-1).url, "/companies/company/reconciliation-groups");
        assert.equal(calls.at(-1).withCredentials, true);
    }
    await payments.listReconciliationGroups({ company_id: "company", include_revoked: true, cursor: "page2" });
    assert.deepEqual(calls.at(-1).params, { limit: 100, cursor: "page2", include_revoked: true });
    await payments.revokeReconciliationGroup({ company_id: "company", group_id: "group", version: "4", idempotency_key: "revoke-key" });
    assert.equal(calls.at(-1).url, "/companies/company/reconciliation-groups/group/revoke");
    assert.deepEqual(body(), { version: "4" });
});
