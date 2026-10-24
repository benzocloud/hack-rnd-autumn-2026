import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import axios from "axios";


test("mock: all endpoints, persisted mutations, auth, jobs, cancellation and no network or automatic CSRF", async () => {
    process.env.developerMode = "mock";

    const storage = new Map();

    global.window = { location: { pathname: "/login" }, localStorage: { getItem: ( key ) => storage.get(key) ?? null, setItem: ( key, value ) => storage.set(key, value) } };
    axios.defaults.adapter = async () => { assert.fail("Mock accessed the real transport"); };

    const { default: api } = await import("../src/service/api.ts");
    const services = await import("../src/service/index.ts");
    const routes = [ ...JSON.parse(readFileSync("src/mock/responses.json", "utf8")), ...JSON.parse(readFileSync("src/mock/authResponses.json", "utf8")) ];
    const visited = new Set();
    let csrfCalls = 0;

    const adapter = api.defaults.adapter;

    api.defaults.adapter = async ( config ) => {
        if (config.url === "/auth/csrf") {
            csrfCalls++;
        }

        assert.ok(!config.headers.get("X-CSRF-Token"));

        const segments = config.url.split("/").filter(Boolean);
        const route = routes.find(( item ) => {
            const expected = item.path.split("/").filter(Boolean);

            return item.method === config.method.toUpperCase() && expected.length === segments.length && expected.every(( part, index ) => part.startsWith("{") || part === segments[index]);
        });

        if (route) {
            visited.add(`${route.method} ${route.path}`);
        }

        return adapter(config);
    };

    const get = async ( url, params ) => (await api.get(url, { params })).data;
    let sequence = 0;
    const write = async ( method, url, data, params, key = `action-${sequence++}` ) => (await api.request({ method, url, data, params, headers: { "Idempotency-Key": key } })).data;

    await assert.rejects(get("/me"), ( error ) => error.response.status === 401);
    await assert.rejects(services.accountService.checkSession(), ( error ) => error.response.status === 401);

    const consent = await services.authService.getPersonalDataConsentDocument();

    await services.authService.register({ data: { email: "demo@example.com", password: "demo-password", display_name: "Демо", personal_data_consent: { accepted: true, document_version: consent.document_version, document_sha256: consent.document_sha256 } } });
    await assert.rejects(services.authService.verifyEmail({ data: { token: "1234" } }), ( error ) => error.response.status === 400);
    await services.authService.resendVerification({ data: { email: "demo@example.com" } });
    await services.authService.verifyEmail({ data: { token: "012345" } });
    assert.equal((await services.accountService.getMe()).email, "demo@example.com");
    assert.equal((await services.accountService.getMe()).email_verified, true);
    await assert.rejects(services.authService.verifyEmail({ data: { token: "012345" } }), ( error ) => error.response.status === 400);

    const session = await services.authService.login({ data: { email: "demo@example.com", password: "" } });

    await assert.rejects(get("/me"), ( error ) => error.response.status === 401);
    await assert.rejects(services.authService.confirmMockLogin({ data: { token: "1234" } }), ( error ) => error.response.status === 400);
    await services.authService.confirmMockLogin({ data: { token: "012345" } });
    assert.equal(session.user.email_verified, true);
    assert.equal((await get("/me")).display_name, "Демо");
    assert.equal((await services.accountService.checkSession()).id, session.user.id);
    assert.ok(Date.parse(session.expires_at) > Date.now());
    await services.authService.forgotPassword({ data: { email: "demo@example.com" } });
    await assert.rejects(services.authService.resetPassword({ data: { token: "012345", new_password: "new-demo-password" } }), ( error ) => error.response.status === 400);
    await assert.rejects(services.authService.confirmMockPasswordReset({ data: { token: "1234" } }), ( error ) => error.response.status === 400);
    await services.authService.confirmMockPasswordReset({ data: { token: "012345" } });
    await services.authService.resetPassword({ data: { token: "012345", new_password: "new-demo-password" } });
    await assert.rejects(get("/me"), ( error ) => error.response.status === 401);
    await services.authService.login({ data: { email: "demo@example.com", password: "" } });
    await services.authService.confirmMockLogin({ data: { token: "012345" } });
    assert.ok(!storage.values().next().value.includes("new-demo-password"));

    const companyData = { name: "Новая", inn: null, currency: "RUB", timezone: "Europe/Moscow" };
    const company = await write("POST", "/companies", companyData, undefined, "same-company");
    const repeated = await write("POST", "/companies", companyData, undefined, "same-company");
    const base = `/companies/${company.id}`;

    assert.equal(repeated.id, company.id);
    assert.equal((await get("/companies")).items.filter(( item ) => item.id === company.id).length, 1);
    assert.equal((await get("/companies", { limit: 1 })).next_cursor, "1");
    assert.equal((await get("/companies", { limit: 1, cursor: "1" })).items.length, 1);
    await write("PATCH", base, { name: "Обновлена", version: company.version });
    assert.equal((await get(base)).name, "Обновлена");
    await assert.rejects(write("PATCH", base, { name: "Ошибка", version: company.version }), ( error ) => error.response.status === 409);

    let account = await get(`${base}/account`);

    account = await write("PUT", `${base}/account/balance`, { balance: { amount_minor: "123456789012345678", currency: "RUB" }, as_of: "2026-10-24", expected_data_version: account.data_version, note: "demo" });
    assert.equal((await get(`${base}/account`)).balance.amount_minor, "123456789012345678");
    await get(`${base}/categories`);

    let transaction = await write("POST", `${base}/transactions`, { operation_date: "2026-10-25", direction: "outflow", amount: { amount_minor: "10000", currency: "RUB" }, counterparty_name: "Демо", payment_purpose: "Оплата", expected_data_version: account.data_version });

    assert.equal((await get(`${base}/transactions`, { direction: "inflow" })).items.length, 0);
    transaction = await write("PATCH", `${base}/transactions/${transaction.id}/category`, { category_id: "cat_rent", version: transaction.version });
    assert.equal((await get(`${base}/transactions`, { from: "2026-10-25", to: "2026-10-25", category_id: "cat_rent" })).items[0].id, transaction.id);

    let payment = await write("POST", `${base}/payments`, { direction: "outflow", amount: { amount_minor: "20000", currency: "RUB" }, scheduled_date: "2026-10-25", counterparty_name: "Демо", description: "Оплата" });

    payment = await write("PATCH", `${base}/payments/${payment.id}`, { description: "Новая", version: payment.version });
    assert.equal((await get(`${base}/payments/${payment.id}`)).description, "Новая");
    assert.equal((await get(`${base}/payments`, { status: "planned" })).items.length, 1);

    const match = await write("POST", `${base}/payments/${payment.id}/matches`, { transaction_id: transaction.id, amount: { amount_minor: "10000", currency: "RUB" }, payment_version: payment.version, transaction_version: transaction.version });

    assert.equal((await get(`${base}/payments/${payment.id}/matches`)).items[0].id, match.id);
    payment = await get(`${base}/payments/${payment.id}`);
    assert.equal(payment.remaining_amount.amount_minor, "10000");
    await write("DELETE", `${base}/payments/${payment.id}/matches/${match.id}`, undefined, { payment_version: payment.version });
    assert.equal((await get(`${base}/payments/${payment.id}`)).remaining_amount.amount_minor, "20000");
    await get(`${base}/members`);

    let invitation = await write("POST", `${base}/invitations`, { email: "demo@example.com" });

    await get(`${base}/invitations`);
    assert.equal((await get("/me/invitations")).items[0].id, invitation.id);
    assert.equal((await write("POST", `/me/invitations/${invitation.id}/accept`, { version: invitation.version })).id, company.id);
    invitation = await write("POST", `${base}/invitations`, { email: "demo@example.com" });
    assert.equal((await write("POST", `/me/invitations/${invitation.id}/decline`, { version: invitation.version })).status, "declined");
    invitation = await write("POST", `${base}/invitations`, { email: "demo@example.com" });
    await write("DELETE", `${base}/invitations/${invitation.id}`);
    await write("DELETE", `${base}/members/${session.user.id}`);
    assert.equal((await get(`${base}/members`)).items.length, 0);

    const providers = await get(`${base}/external-providers`);
    const connection = await write("POST", `${base}/connections`, { provider_id: providers.items[0].id, display_name: "Демо", source_company_id: "C007" });

    assert.equal((await get(`${base}/connections`)).items[0].id, connection.id);

    const synced = await write("POST", `${base}/connections/${connection.id}/sync`, { period_start: "2026-10-01", period_end: "2026-10-24", record_kinds: [ "transactions", "planned_payments" ] });
    const preview = await get(`${base}/imports/${synced.id}`);

    assert.equal(preview.status, "preview_ready");
    account = await get(`${base}/account`);
    await write("POST", `${base}/imports/${synced.id}/commit`, { version: preview.version, expected_data_version: account.data_version, acknowledge_warnings: true });
    assert.equal((await get(`${base}/imports/${synced.id}`)).status, "applied");
    assert.equal((await get(`${base}/transactions`)).items.length, 2);

    const form = new FormData();

    form.set("file", new File([ "csv" ], "demo.csv"));
    form.set("account_id", account.id);
    form.set("source_company_id", "C001");

    const imported = await write("POST", `${base}/imports`, form);

    await get(`${base}/imports`);
    await get(`${base}/imports/${imported.id}/issues`);
    await write("DELETE", `${base}/connections/${connection.id}`);
    assert.equal((await get(`${base}/connections`)).items[0].status, "disabled");

    const run = await write("POST", `${base}/forecasts`, { cutoff_date: "2026-10-24", horizon_days: 30 });
    const forecast = await get(`${base}/forecasts/${run.id}`);

    assert.equal(forecast.status, "succeeded");
    assert.equal(forecast.result.daily_points.length, 30);

    const scenarioRun = await write("POST", `${base}/scenarios`, { name: "Расход", base_forecast_id: forecast.id, changes: [ { id: "expense", type: "one_off_expense", date: "2026-10-26", amount: { amount_minor: "1000", currency: "RUB" }, description: "Демо" } ] });
    const scenario = await get(`${base}/scenarios/${scenarioRun.id}`);

    await get(`${base}/scenarios`, { base_forecast_id: forecast.id });
    assert.equal((await get(`${base}/forecasts/${scenarioRun.forecast_id}/comparison`, { reference_forecast_id: forecast.id })).delta_closing_balance.amount_minor, "-1000");
    await write("POST", `${base}/scenarios`, { name: "Перенос", comparison_parent_scenario_id: scenario.id, response_action: { type: "reschedule_payment_part", target: { effective_occurrence_id: `eff_component_${payment.id}` }, amount: { amount_minor: "1000", currency: "RUB" }, new_date: "2026-11-01", feasibility_condition: "Демо" } });
    assert.equal((await get(`${base}/forecasts`, { include_scenarios: false })).items.length, 1);
    assert.equal((await get(`${base}/forecasts`, { include_scenarios: true })).items.length, 3);
    assert.equal((await get(`${base}/forecasts/${forecast.id}/plan-fact`, { from: "2026-10-25", to: "2026-10-25" })).daily_points.length, 1);
    payment = await get(`${base}/payments/${payment.id}`);
    await write("DELETE", `${base}/payments/${payment.id}`, undefined, { version: payment.version });
    assert.equal((await get(`${base}/payments/${payment.id}`)).status, "cancelled");
    assert.equal((await get(`${base}/forecasts/${forecast.id}`)).is_stale, true);

    const controller = new AbortController();
    const pending = api.post("/companies", companyData, { signal: controller.signal });

    setTimeout(() => controller.abort(), 30);
    await assert.rejects(pending, ( error ) => axios.isCancel(error));
    assert.equal((await get("/companies")).items.length, 2);
    await assert.rejects(get("/companies/does-not-exist"), ( error ) => error.response.status === 404);
    for (const days of [ 30, 60, 90 ]) {
        const dashboard = await services.dashboardService.getDashboardData(days, new AbortController().signal);

        assert.equal(dashboard.company.id, company.id);
        assert.equal(dashboard.forecast.input.horizon_days, days);
        assert.equal(dashboard.forecast.result.daily_points.length, days);
    }

    const cachedDashboard = await services.dashboardService.getDashboardData(30, new AbortController().signal);
    const reusedDashboard = await services.dashboardService.getDashboardData(30, new AbortController().signal);

    assert.equal(cachedDashboard.forecast.id, reusedDashboard.forecast.id);
    await services.authService.logout();
    await assert.rejects(get("/me"), ( error ) => error.response.status === 401);
    assert.equal(csrfCalls, 0, "No automatic CSRF calls in mock");
    await services.authService.getCsrfToken();
    assert.equal(visited.size, 56, JSON.stringify(routes.filter(( item ) => !visited.has(`${item.method} ${item.path}`))));
});
