import type { IComparison, ICreateForecastRequest, ICreateScenarioRequest, IForecastComponent, IForecastSucceeded, IPlanFact, IScenario } from "@/interfaces";

import { findItem, inPeriod, mockError, mockId, now, paginate } from "./helpers";
import { fixture, type IMockCompanyData } from "./state";


const addDays = ( date: string, days: number ) => new Date(Date.parse(`${date}T00:00:00Z`) + days * 86400000).toISOString().slice(0, 10);


const money = ( value: bigint ) => ({ amount_minor: String(value), currency: "RUB" as const });


const calculateForecast = ( forecast: IForecastSucceeded ) => {
    let balance = BigInt(forecast.input.opening_balance.amount_minor);
    let minimum = balance;

    forecast.result.metrics.first_gap = null;
    forecast.result.daily_points = Array.from({ length: forecast.input.horizon_days }, ( _, index ) => {
        const date = addDays(forecast.input.start_date, index);
        const components = forecast.result.components.filter(( component ) => component.date === date);
        const inflow = components.filter(( item ) => item.direction === "inflow").reduce(( sum, item ) => sum + BigInt(item.remaining_amount.amount_minor), BigInt(0));
        const outflow = components.filter(( item ) => item.direction === "outflow").reduce(( sum, item ) => sum + BigInt(item.remaining_amount.amount_minor), BigInt(0));

        balance += inflow - outflow;
        minimum = balance < minimum ? balance : minimum;

        if (balance < BigInt(0) && !forecast.result.metrics.first_gap) {
            forecast.result.metrics.first_gap = { date, shortfall: money(-balance) };
        }

        return { date, inflow: money(inflow), outflow: money(outflow), balance: money(balance), balance_lower: null, balance_upper: null, negative_balance_probability: { value: null, unavailable_reason: "not_estimated" } as const };
    });

    forecast.result.metrics.minimum_balance = money(minimum);
    forecast.result.metrics.max_shortfall = money(minimum < BigInt(0) ? -minimum : BigInt(0));
    forecast.result.metrics.closing_balance = money(balance);
    forecast.result.effective_occurrences = forecast.result.components.map(( component ) => ({
        effective_occurrence_id: `eff_${component.id}`,
        component_id: component.id,
        occurrence_id: component.occurrence_id,
        payment_id: component.payment_id,
        direction: component.direction,
        date: component.date,
        remaining_amount: { ...component.remaining_amount },
        in_horizon: component.date >= forecast.input.start_date && component.date <= forecast.input.end_date,
        response_action_eligible: component.direction === "outflow",
    }));
};


const createForecast = ( company: IMockCompanyData, request: ICreateForecastRequest ) => {
    const forecast = fixture<IForecastSucceeded>("ForecastSucceeded");

    if (!/^\d{4}-\d{2}-\d{2}$/.test(request.cutoff_date) || !Number.isFinite(Date.parse(request.cutoff_date)) || !Number.isInteger(request.horizon_days) || request.horizon_days < 30 || request.horizon_days > 90) {
        throw mockError(400, "Укажите дату прогноза и горизонт от 30 до 90 дней.");
    }

    forecast.id = mockId("fc");
    forecast.company_id = company.account.company_id;
    forecast.created_at = now();
    forecast.completed_at = now();
    forecast.current_data_version = company.account.data_version;
    forecast.is_stale = false;
    forecast.input = { ...forecast.input, ...request, snapshot_id: mockId("snap"), data_version: company.account.data_version, account_id: company.account.id, opening_balance: company.account.balance || money(BigInt(0)), balance_as_of: company.account.balance_as_of || request.cutoff_date, data_as_of: request.cutoff_date, start_date: addDays(request.cutoff_date, 1), end_date: addDays(request.cutoff_date, request.horizon_days) };
    forecast.result.components = company.payments.filter(( payment ) => payment.status !== "cancelled" && payment.status !== "matched").map(( payment ): IForecastComponent => ({
        id: `component_${payment.id}`,
        occurrence_id: `occurrence_${payment.id}`,
        kind: "known_payment",
        direction: payment.direction,
        date: payment.scheduled_date,
        amount: { ...payment.amount },
        remaining_amount: { ...payment.remaining_amount },
        category_id: payment.category_id,
        payment_id: payment.id,
        recurrence_group_id: null,
        source: payment.source,
        scenario_eligible: true,
    }));
    forecast.result.assumptions = [ "Демонстрационный расчёт по запланированным платежам. ML-модель в режиме mock не вызывается." ];
    forecast.result.recommendations = [];
    calculateForecast(forecast);
    company.forecasts.unshift(forecast);

    return forecast;
};


const createScenario = ( company: IMockCompanyData, request: ICreateScenarioRequest ) => {
    const parent = "comparison_parent_scenario_id" in request ? findItem(company.scenarios, request.comparison_parent_scenario_id) : null;
    const baseId = parent?.base_forecast_id || ("base_forecast_id" in request ? request.base_forecast_id : "");
    const forecast = structuredClone(findItem(company.forecasts, parent?.forecast_id || baseId));
    const scenario: IScenario = { ...fixture<IScenario>("Scenario"), id: mockId("sc"), name: request.name, base_forecast_id: baseId, comparison_parent_scenario_id: parent?.id || null, forecast_id: mockId("fc"), changes: "changes" in request ? request.changes : [], response_action: "response_action" in request ? request.response_action : null, out_of_horizon_occurrence_ids: [], created_at: now() };

    for (const change of scenario.changes) {
        if (change.type === "delay_receipt") {
            const component = forecast.result.components.find(( item ) => item.id === change.target.component_id && item.occurrence_id === change.target.occurrence_id);

            if (!component) {
                throw mockError(404, "Поступление для сценария не найдено.");
            }

            const amount = BigInt(change.amount.amount_minor);

            if (component.direction !== "inflow" || amount <= BigInt(0) || amount > BigInt(component.remaining_amount.amount_minor)) {
                throw mockError(400, "Проверьте сумму переносимого поступления.");
            }

            component.remaining_amount = money(BigInt(component.remaining_amount.amount_minor) - amount);
            forecast.result.components.push({ ...structuredClone(component), id: mockId("component"), occurrence_id: mockId("occurrence"), date: change.new_date, amount: money(amount), remaining_amount: money(amount) });
        } else if (change.type === "one_off_expense") {
            forecast.result.components.push({ id: mockId("component"), occurrence_id: mockId("occurrence"), kind: "scenario_expense", direction: "outflow", date: change.date, amount: change.amount, remaining_amount: change.amount, category_id: change.category_id || null, payment_id: null, recurrence_group_id: null, source: null, scenario_eligible: true });
        } else {
            throw mockError(400, "Этот тип изменения недоступен для демонстрационного прогноза.");
        }
    }

    if (scenario.response_action) {
        const action = scenario.response_action;
        const occurrence = forecast.result.effective_occurrences.find(( item ) => item.effective_occurrence_id === action.target.effective_occurrence_id);
        const component = forecast.result.components.find(( item ) => item.id === occurrence?.component_id);
        const amount = BigInt(action.amount.amount_minor);

        if (!component || component.direction !== "outflow" || amount <= BigInt(0) || amount > BigInt(component.remaining_amount.amount_minor)) {
            throw mockError(400, "Проверьте платёж и сумму переноса.");
        }

        component.remaining_amount = money(BigInt(component.remaining_amount.amount_minor) - amount);
        forecast.result.components.push({ ...structuredClone(component), id: mockId("component"), occurrence_id: mockId("occurrence"), date: action.new_date, amount: money(amount), remaining_amount: money(amount) });
    }

    forecast.id = scenario.forecast_id;
    forecast.scenario_id = scenario.id;
    forecast.created_at = now();
    forecast.completed_at = now();
    calculateForecast(forecast);
    scenario.out_of_horizon_occurrence_ids = forecast.result.effective_occurrences.filter(( item ) => !item.in_horizon).map(( item ) => item.occurrence_id);
    company.scenarios.unshift(scenario);
    company.forecasts.unshift(forecast);

    return { id: scenario.id, forecast_id: forecast.id, status: "queued", status_url: `/api/v1/companies/${company.account.company_id}/forecasts/${forecast.id}`, poll_after_ms: 300 };
};


const handleMockForecasts = ( method: string, segments: string[], data: unknown, params: Record<string, unknown>, company: IMockCompanyData ): unknown => {
    const [ , , resource, id, action ] = segments;

    if (resource === "scenarios") {
        if (method === "POST") {
            return createScenario(company, data as ICreateScenarioRequest);
        }

        return id ? findItem(company.scenarios, id) : paginate(company.scenarios.filter(( scenario ) => !params.base_forecast_id || scenario.base_forecast_id === params.base_forecast_id), params);
    }

    if (method === "POST") {
        const forecast = createForecast(company, data as ICreateForecastRequest);

        return { id: forecast.id, status: "queued", status_url: `/api/v1/companies/${company.account.company_id}/forecasts/${forecast.id}`, poll_after_ms: 300 };
    }

    if (!id) {
        return paginate(company.forecasts.filter(( forecast ) => params.include_scenarios === true || params.include_scenarios === "true" || !forecast.scenario_id).map(( forecast ) => ({ id: forecast.id, status: forecast.status, cutoff_date: forecast.input.cutoff_date, start_date: forecast.input.start_date, end_date: forecast.input.end_date, created_at: forecast.created_at, is_stale: forecast.is_stale, scenario_id: forecast.scenario_id })), params);
    }

    const forecast = findItem(company.forecasts, id);

    if (action === "comparison") {
        const reference = findItem(company.forecasts, String(params.reference_forecast_id));
        const result = fixture<IComparison>("Comparison");

        result.reference_forecast_id = reference.id;
        result.candidate_forecast_id = forecast.id;
        result.reference_metrics = reference.result.metrics;
        result.candidate_metrics = forecast.result.metrics;
        result.snapshot_id = forecast.input.snapshot_id;
        result.model_context_id = forecast.input.model_context.id;
        result.is_stale = reference.is_stale || forecast.is_stale;
        result.delta_max_shortfall = money(BigInt(forecast.result.metrics.max_shortfall.amount_minor) - BigInt(reference.result.metrics.max_shortfall.amount_minor));
        result.delta_minimum_balance = money(BigInt(forecast.result.metrics.minimum_balance.amount_minor) - BigInt(reference.result.metrics.minimum_balance.amount_minor));
        result.delta_closing_balance = money(BigInt(forecast.result.metrics.closing_balance.amount_minor) - BigInt(reference.result.metrics.closing_balance.amount_minor));
        result.delta_gap_probability = forecast.result.metrics.gap_probability.value === null || reference.result.metrics.gap_probability.value === null ? null : forecast.result.metrics.gap_probability.value - reference.result.metrics.gap_probability.value;
        result.delta_first_gap_days = reference.result.metrics.first_gap && forecast.result.metrics.first_gap ? Math.round((Date.parse(forecast.result.metrics.first_gap.date) - Date.parse(reference.result.metrics.first_gap.date)) / 86400000) : null;

        return result;
    }

    if (action === "plan-fact") {
        const result = fixture<IPlanFact>("PlanFact");

        result.forecast_id = forecast.id;
        result.actual_data_version = company.account.data_version;
        result.daily_points = forecast.result.daily_points.filter(( point ) => inPeriod(point.date, params)).map(( point ) => {
            const transactions = company.transactions.filter(( item ) => item.operation_date === point.date);
            const inflow = transactions.filter(( item ) => item.direction === "inflow").reduce(( sum, item ) => sum + BigInt(item.amount.amount_minor), BigInt(0));
            const outflow = transactions.filter(( item ) => item.direction === "outflow").reduce(( sum, item ) => sum + BigInt(item.amount.amount_minor), BigInt(0));

            return { date: point.date, planned_inflow: point.inflow, planned_outflow: point.outflow, planned_balance: point.balance, actual_inflow: transactions.length ? money(inflow) : null, actual_outflow: transactions.length ? money(outflow) : null, actual_balance: null, balance_delta: null, coverage: transactions.length ? "partial" : "unavailable" };
        });
        result.period_start = result.daily_points[0]?.date || forecast.input.start_date;
        result.period_end = result.daily_points.at(-1)?.date || forecast.input.end_date;
        result.coverage.missing_dates = result.daily_points.filter(( point ) => point.coverage === "unavailable").map(( point ) => point.date);
        result.coverage.observed_through = result.daily_points.filter(( point ) => point.coverage === "partial").at(-1)?.date || null;
        result.coverage.status = result.coverage.observed_through ? "partial" : "unavailable";

        result.mean_absolute_balance_error = null;

        return result;
    }

    return forecast;
};


export default handleMockForecasts;
