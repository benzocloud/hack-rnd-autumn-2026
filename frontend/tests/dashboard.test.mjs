import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import dashboardService from "../src/service/dashboard_service/dashboard_service.ts";
import companiesService from "../src/service/companies_service/companies_service.ts";
import forecastsService from "../src/service/forecasts_service/forecasts_service.ts";
import ledgerService from "../src/service/ledger_service/ledger_service.ts";


const bodies = JSON.parse(readFileSync(new URL("../src/mock/bodies.json", import.meta.url), "utf8"));


const setup = ( t ) => {
    const forecast = structuredClone(bodies.ForecastSucceeded);
    const company = { id: forecast.company_id };
    const account = { balance: forecast.input.opening_balance, balance_as_of: forecast.input.cutoff_date };
    const summary = { id: forecast.id, cutoff_date: forecast.input.cutoff_date, end_date: forecast.input.end_date, is_stale: false, status: "succeeded" };

    t.mock.method(companiesService, "getCompany", async () => company);
    t.mock.method(ledgerService, "getDefaultAccount", async () => account);
    t.mock.method(forecastsService, "listForecasts", async () => ({ items: [ summary ], next_cursor: null }));
    t.mock.method(forecastsService, "getForecast", async () => forecast);
    const create = t.mock.method(forecastsService, "createForecast", async () => ({ id: forecast.id, poll_after_ms: 1000 }));

    return { forecast, company, summary, create };
};


test("dashboard searches all forecast pages and reuses the saved result", async ( t ) => {
    const { forecast, company, summary, create } = setup(t);
    const list = t.mock.method(forecastsService, "listForecasts", async ( request ) => request.cursor ? { items: [ summary ], next_cursor: null } : { items: [], next_cursor: "next" });
    const result = await dashboardService.getDashboardData(30, new AbortController().signal, company.id);

    assert.equal(result.forecast, forecast);
    assert.equal(list.mock.calls[1].arguments[0].cursor, "next");
    assert.equal(create.mock.callCount(), 0);
});


test("home dashboard resolves the company from current server memberships", async ( t ) => {
    const { company, forecast } = setup(t);
    const memberships = t.mock.method(companiesService, "listCompanies", async () => ({ items: [ company ], next_cursor: null }));
    const profile = t.mock.method(companiesService, "getCompany", async () => { throw new Error("stale company must not be requested"); });
    const result = await dashboardService.getDashboardData(30, new AbortController().signal);

    assert.equal(result.company, company);
    assert.equal(result.forecast, forecast);
    assert.equal(memberships.mock.callCount(), 1);
    assert.equal(profile.mock.callCount(), 0);
});


test("explicit company routes preserve access failures without silently switching companies", async ( t ) => {
    setup(t);
    const denied = new Error("Company not found");
    t.mock.method(companiesService, "getCompany", async () => { throw denied; });
    const memberships = t.mock.method(companiesService, "listCompanies", async () => ({ items: [], next_cursor: null }));

    await assert.rejects(dashboardService.getDashboardData(30, new AbortController().signal, "foreign-company"), ( error ) => error === denied);
    assert.equal(memberships.mock.callCount(), 0);
});


test("dashboard reports queued/running/succeeded and polls the accepted forecast", async ( t ) => {
    const { forecast, company } = setup(t);
    let reads = 0;
    const states = [];
    t.mock.method(forecastsService, "getForecast", async () => ++reads < 3 ? { ...forecast, status: reads === 1 ? "queued" : "running", result: null } : forecast);

    await dashboardService.getDashboardData(30, new AbortController().signal, company.id, ( status ) => states.push(status));

    assert.deepEqual(states, [ "queued", "running", "succeeded" ]);
});


test("dashboard rejects missing or duplicated forecast dates without filling zeros", async ( t ) => {
    const { forecast, company } = setup(t);
    const original = structuredClone(forecast.result.daily_points);
    forecast.result.daily_points.pop();
    await assert.rejects(dashboardService.getDashboardData(30, new AbortController().signal, company.id), /неполный дневной прогноз/);
    assert.equal(forecast.result.daily_points.length, 29);

    forecast.result.daily_points = original;
    forecast.result.daily_points[1].date = forecast.result.daily_points[0].date;
    await assert.rejects(dashboardService.getDashboardData(30, new AbortController().signal, company.id), /неполный дневной прогноз/);
});


test("dashboard refreshes stale input and preserves server staleness on the result", async ( t ) => {
    const { forecast, company, summary, create } = setup(t);
    summary.is_stale = true;
    forecast.is_stale = true;

    const result = await dashboardService.getDashboardData(30, new AbortController().signal, company.id);

    assert.equal(create.mock.callCount(), 1);
    assert.equal(result.forecast.is_stale, true);
});


test("dashboard stops polling on cancellation and propagates terminal failures", async ( t ) => {
    const { forecast, company } = setup(t);
    const controller = new AbortController();
    t.mock.method(forecastsService, "getForecast", async () => ({ ...forecast, status: "queued", result: null }));
    await assert.rejects(dashboardService.getDashboardData(30, controller.signal, company.id, () => controller.abort()), ( error ) => error.code === "ERR_CANCELED");

    t.mock.method(forecastsService, "getForecast", async () => ({ ...forecast, status: "failed", result: null, error: { message: "ML недоступен" } }));
    await assert.rejects(dashboardService.getDashboardData(30, new AbortController().signal, company.id), /ML недоступен/);
});
