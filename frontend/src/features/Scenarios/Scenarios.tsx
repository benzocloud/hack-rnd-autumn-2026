"use client";

import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { CreateForecastButton } from "@/features/ForecastDetail/ForecastDetail";
import { addDays, companyPath, dateField, formatDate, formIdentity, invalid, parseMoney, textField } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import { collectPages } from "@/helpers/pagination";
import useJobResource from "@/hooks/useJobResource";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IForecastSucceeded, IScenario, IScenarioChange, IScenarioCapabilities } from "@/interfaces";
import { forecastsService, scenariosService } from "@/service";
import { BalanceChart } from "@/widgets";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, JobState, LinkButton, Modal, Notice, PageHeading, Panel, Select, TextArea } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


type Template = Exclude<keyof IScenarioCapabilities, "unavailable_reason">;


const templates: { type: Template; title: string; description: string }[] = [
    { type: "delay_receipt", title: "Задержка поступления", description: "Что будет, если клиент заплатит позже?" },
    { type: "reduce_sales", title: "Снижение выручки", description: "Уменьшите модельные поступления на выбранный процент." },
    { type: "one_off_expense", title: "Разовый расход", description: "Проверьте, как крупная покупка повлияет на остаток." },
    { type: "increase_recurring_expenses", title: "Рост постоянных расходов", description: "Оцените увеличение регулярных затрат." },
];


const Scenarios = () => {
    const { company } = useCompany();
    const [ baseId, setBaseId ] = useState("");
    const [ selected, setSelected ] = useState("");
    const resource = useResource(useCallback(async ( signal: AbortSignal ) => {
        const [ forecasts, scenarios ] = await Promise.all([
            collectPages(( cursor ) => forecastsService.listForecasts({ company_id: company.id, include_scenarios: false, limit: 100, cursor }, signal)),
            collectPages(( cursor ) => scenariosService.listScenarios({ company_id: company.id, limit: 100, cursor }, signal)),
        ]);

        return { forecasts, scenarios };
    }, [ company.id ]));
    const chosenId = baseId || resource.data?.forecasts.find(( item ) => item.status === "succeeded")?.id || "";


    return <main className={styles.page}><PageHeading title="Сценарии «что если»" description="Проверяйте решения на сохранённом прогнозе. Сценарии не меняют операции и платёжный календарь."><CreateForecastButton /></PageHeading><div className={styles.stack}><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && <><Panel title="Исходный прогноз">{ resource.data.forecasts.length ? <Select label="База для новых сценариев" value={chosenId} onChange={( event ) => { setBaseId(event.target.value); setSelected(""); }}><option value="">Выберите завершённый прогноз</option>{ resource.data.forecasts.filter(( item ) => item.status === "succeeded").map(( item ) => <option key={item.id} value={item.id}>{ formatDate(item.start_date) } — { formatDate(item.end_date) } · создан { formatDate(item.created_at) }{ item.is_stale ? " · данные изменились" : "" }</option>) }</Select> : <EmptyState title="Сначала нужен прогноз" icon="chart-line" action={<CreateForecastButton />}>Укажите остаток на счёте и постройте прогноз. Он станет базой для сравнения вариантов.</EmptyState> }</Panel>{ chosenId && <ScenarioTemplates key={chosenId} forecastId={chosenId} onCreated={( id ) => { setSelected(id); resource.reload(); }} /> }<Panel title="Сохранённые варианты">{ resource.data.scenarios.length ? resource.data.scenarios.map(( item ) => <div className={styles.item} key={item.id}><div><h3>{ item.name }</h3><p className={styles.muted}>{ formatDate(item.created_at) } · { item.comparison_parent_scenario_id ? "Стресс + ответное действие" : "Сценарий" }</p></div><ActionButton variant="secondary" onClick={() => setSelected(item.id)}>Сравнить</ActionButton></div>) : <EmptyState title="Сценариев пока нет" icon="adjustments">Выберите подходящий шаблон, чтобы проверить первый вариант.</EmptyState> }</Panel></> }</DataState>{ selected && <ScenarioComparison key={selected} id={selected} onCreated={( id ) => { setSelected(id); resource.reload(); }} /> }</div></main>;
};


const ScenarioTemplates = ( { forecastId, onCreated }: { forecastId: string; onCreated: ( id: string ) => void } ) => {
    const { company } = useCompany();
    const [ template, setTemplate ] = useState<Template | null>(null);
    const resource = useResource(useCallback(( signal: AbortSignal ) => forecastsService.getForecast({ company_id: company.id, forecast_id: forecastId }, signal), [ company.id, forecastId ]));
    const forecast = resource.data?.status === "succeeded" ? resource.data : null;


    return <DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ forecast && <div className={styles.stack}>{ forecast.is_stale && <Notice tone="warning">Выбранный прогноз использует прежние данные. Сценарии будут сравниваться с тем же сохранённым снимком.</Notice> }<div className={styles.grid}>{ templates.map(( item ) => <Panel key={item.type} title={item.title}><p>{ item.description }</p>{ !forecast.result.scenario_capabilities[item.type] && <p className={styles.muted}>{ forecast.result.scenario_capabilities.unavailable_reason || "В выбранном прогнозе нет подходящих компонентов." }</p> }<ActionButton variant="secondary" disabled={!forecast.result.scenario_capabilities[item.type]} onClick={() => setTemplate(item.type)}>Настроить вариант</ActionButton></Panel>) }</div>{ template && <ScenarioForm key={template} template={template} forecast={forecast} onClose={() => setTemplate(null)} onCreated={( id ) => { setTemplate(null); onCreated(id); }} /> }</div> }</DataState>;
};


const ScenarioForm = ( { template, forecast, onClose, onCreated }: { template: Template; forecast: IForecastSucceeded; onClose: () => void; onCreated: ( id: string ) => void } ) => {
    const { company } = useCompany();
    const request = useRequest();
    const eligible = forecast.result.components.filter(( component ) => component.scenario_eligible && BigInt(component.remaining_amount.amount_minor) > BigInt(0) && component.date >= forecast.input.start_date && component.date <= forecast.input.end_date && (template === "delay_receipt" ? component.direction === "inflow" : template === "reduce_sales" ? component.kind === "ml_sales_receipt" : component.kind === "recurring_expense"));
    const title = templates.find(( item ) => item.type === template)?.title || "Новый сценарий";


    const save = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const name = textField(form, "name", "Название сценария");
            const id = crypto.randomUUID();
            let change: IScenarioChange;

            if (template === "one_off_expense") {
                const date = dateField(form, "date", "Дата расхода");

                if (date < forecast.input.start_date || date > forecast.input.end_date) { return invalid("Дата расхода должна входить в период прогноза.", "date"); }
                change = { id, type: template, date, amount: { amount_minor: parseMoney(textField(form, "amount", "Сумма")), currency: "RUB" }, description: textField(form, "description", "Описание", 1000) };
            } else if (template === "delay_receipt") {
                const component = eligible.find(( item ) => item.id === form.get("target"));
                const date = dateField(form, "date", "Новая дата");
                const amount = parseMoney(textField(form, "amount", "Сумма"));

                if (!component) { return invalid("Выберите поступление.", "target"); }
                if (date <= component.date) { return invalid("Новая дата должна быть позже исходной даты поступления.", "date"); }
                if (BigInt(amount) > BigInt(component.remaining_amount.amount_minor)) { return invalid("Сумма превышает доступную часть поступления.", "amount"); }
                change = { id, type: template, target: { component_id: component.id, occurrence_id: component.occurrence_id }, new_date: date, amount: { amount_minor: amount, currency: "RUB" } };
            } else {
                const selected = form.getAll("targets").map(String);
                const targets = eligible.filter(( item ) => selected.includes(item.id)).map(( item ) => ({ component_id: item.id, occurrence_id: item.occurrence_id }));
                const percent = Number(String(form.get("percent") || "").replace(",", "."));
                const basisPoints = Math.round(percent * 100);

                if (!targets.length) { return invalid("Выберите хотя бы один компонент прогноза.", "targets"); }
                if (!Number.isFinite(percent) || percent <= 0 || (template === "reduce_sales" && percent > 100) || basisPoints < 1 || basisPoints > 1000000) { return invalid("Укажите корректный процент изменения.", "percent"); }
                if (template === "reduce_sales") {
                    const from = dateField(form, "from", "Начало периода");
                    const to = dateField(form, "to", "Конец периода");

                    if (from > to || from < forecast.input.start_date || to > forecast.input.end_date) { return invalid("Выберите период внутри горизонта прогноза.", "from"); }
                    change = { id, type: template, targets, period_start: from, period_end: to, reduction_basis_points: basisPoints };
                } else {
                    const date = dateField(form, "date", "С какой даты");

                    if (date < forecast.input.start_date || date > forecast.input.end_date) { return invalid("Дата должна входить в период прогноза.", "date"); }
                    change = { id, type: template, targets, effective_date: date, adjustment: { mode: "percent", increase_basis_points: basisPoints } };
                }
            }

            return scenariosService.createScenario({ company_id: company.id, idempotency_key: key, data: { name, base_forecast_id: forecast.id, changes: [ change ] } }, signal);
        }, "Сценарий принят на расчёт", formIdentity(form));

        if (result.ok) { onCreated(result.data.id); }
    };


    return <Modal title={title} busy={request.busy} onClose={onClose}><form className={styles.form} noValidate onSubmit={save}><fieldset disabled={request.busy}><Field name="name" label="Название сценария" defaultValue={title} autoFocus />{ template === "delay_receipt" && <Select disabled={request.busy} name="target" label="Какое поступление перенести"><option value="">Выберите поступление</option>{ eligible.map(( item ) => <option key={item.id} value={item.id}>{ formatDate(item.date) } · { formatMoney(item.remaining_amount.amount_minor) }</option>) }</Select> }{ (template === "delay_receipt" || template === "one_off_expense") && <Field name="amount" label="Сумма, ₽" inputMode="decimal" placeholder="0,00" /> }{ template !== "reduce_sales" && <Field name="date" label={template === "one_off_expense" ? "Дата расхода" : template === "delay_receipt" ? "Новая дата поступления" : "С какой даты"} type="date" defaultValue={forecast.input.start_date} /> }{ template === "one_off_expense" && <TextArea name="description" label="Описание расхода" /> }{ (template === "reduce_sales" || template === "increase_recurring_expenses") && <><fieldset className={styles.targets}><legend>Компоненты прогноза</legend>{ eligible.map(( item ) => <label key={item.id}><input type="checkbox" name="targets" value={item.id} defaultChecked />{ formatDate(item.date) } · { formatMoney(item.remaining_amount.amount_minor) }</label>) }</fieldset><Field name="percent" label={template === "reduce_sales" ? "На сколько снизить, %" : "На сколько увеличить, %"} inputMode="decimal" placeholder="10" />{ template === "reduce_sales" && <><Field name="from" label="Начало периода" type="date" defaultValue={forecast.input.start_date} /><Field name="to" label="Конец периода" type="date" defaultValue={forecast.input.end_date} /></> }</> }</fieldset>{ template === "delay_receipt" && <Notice>Если новая дата окажется за пределами горизонта, поступление не попадёт в остаток этого периода.</Notice> }<FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Рассчитать сценарий</ActionButton></form></Modal>;
};


const ScenarioComparison = ( { id, onCreated }: { id: string; onCreated: ( id: string ) => void } ) => {
    const { company } = useCompany();
    const resource = useResource(useCallback(async ( signal: AbortSignal ) => {
        const scenario = await scenariosService.getScenario({ company_id: company.id, scenario_id: id }, signal);
        const parent = scenario.comparison_parent_scenario_id ? await scenariosService.getScenario({ company_id: company.id, scenario_id: scenario.comparison_parent_scenario_id }, signal) : null;
        const reference = await forecastsService.getForecast({ company_id: company.id, forecast_id: parent?.forecast_id || scenario.base_forecast_id }, signal);

        return { scenario, reference };
    }, [ company.id, id ]));


    return <Panel title="Сравнение вариантов"><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && resource.data.reference.status === "succeeded" && <ComparisonResult scenario={resource.data.scenario} reference={resource.data.reference} onCreated={onCreated} /> }</DataState></Panel>;
};


const ComparisonResult = ( { scenario, reference, onCreated }: { scenario: IScenario; reference: IForecastSucceeded; onCreated: ( id: string ) => void } ) => {
    const { company } = useCompany();
    const [ responding, setResponding ] = useState(false);
    const resource = useJobResource(useCallback(( signal: AbortSignal ) => forecastsService.getForecast({ company_id: company.id, forecast_id: scenario.forecast_id }, signal), [ company.id, scenario.forecast_id ]));
    const ready = resource.data?.status === "succeeded";
    const comparison = useResource(useCallback(( signal: AbortSignal ) => ready ? forecastsService.compareForecasts({ company_id: company.id, forecast_id: scenario.forecast_id, reference_forecast_id: reference.id }, signal) : Promise.resolve(null), [ company.id, scenario.forecast_id, reference.id, ready ]));
    const forecast = resource.data?.status === "succeeded" ? resource.data : null;


    return <DataState loading={resource.loading && !resource.data} error={resource.error} onRetry={resource.reload}>{ resource.data && <div className={styles.stack}><h3>{ scenario.name }</h3>{ resource.data.status === "failed" ? <Notice tone="danger">{ resource.data.error.message || "Сценарий не рассчитан. Создайте новый вариант." }</Notice> : !forecast ? <JobState label="Рассчитываем сценарий" /> : <>{ (forecast.is_stale || reference.is_stale) && <Notice tone="warning">Данные компании изменились. Сравнение показывает варианты на одном сохранённом снимке.</Notice> }<p className={styles.muted}>{ scenario.comparison_parent_scenario_id ? "Сравниваем стресс и стресс с ответным действием. Исходные стрессовые условия сохранены." : "Сравниваем исходный прогноз и выбранный сценарий." }</p><div className={styles.chart}><BalanceChart points={forecast.result.daily_points} comparison={reference.result.daily_points} /></div><p className={styles.muted}>Голубая линия — выбранный сценарий. Белая пунктирная — { scenario.comparison_parent_scenario_id ? "стресс без действия" : "исходный прогноз" }.</p><DataState loading={comparison.loading} error={comparison.error} onRetry={comparison.reload}>{ comparison.data && <div className={styles.grid}><div><h3>Минимальный остаток</h3><p>{ formatMoney(comparison.data.reference_metrics.minimum_balance.amount_minor) } → { formatMoney(comparison.data.candidate_metrics.minimum_balance.amount_minor) }</p><Badge>Изменение { formatMoney(comparison.data.delta_minimum_balance.amount_minor) }</Badge></div><div><h3>Первый отрицательный остаток</h3><p>{ comparison.data.reference_metrics.first_gap ? formatDate(comparison.data.reference_metrics.first_gap.date) : "Не прогнозируется" } → { comparison.data.candidate_metrics.first_gap ? formatDate(comparison.data.candidate_metrics.first_gap.date) : "Не прогнозируется" }</p></div><div><h3>Остаток в конце периода</h3><p>{ formatMoney(comparison.data.candidate_metrics.closing_balance.amount_minor) }</p><p className={styles.muted}>Изменение { formatMoney(comparison.data.delta_closing_balance.amount_minor) }</p></div></div> }</DataState>{ scenario.out_of_horizon_occurrence_ids.length > 0 && <Notice tone="warning">За пределами горизонта осталось частей платежей: { scenario.out_of_horizon_occurrence_ids.length }. Они не исчезли, но не влияют на остаток внутри этого периода.</Notice> }{ scenario.response_action && <Notice>Условие ответного действия: { scenario.response_action.feasibility_condition }</Notice> }<div className={styles.row}><ActionButton disabled={!forecast.result.effective_occurrences.some(( item ) => item.response_action_eligible && item.in_horizon && BigInt(item.remaining_amount.amount_minor) > BigInt(0))} onClick={() => setResponding(true)}>Проверить перенос расхода</ActionButton><LinkButton href={companyPath(company.id, `forecasts/${forecast.id}`)} secondary>Открыть прогноз и план-факт</LinkButton></div>{ responding && <ResponseForm scenario={scenario} forecast={forecast} onClose={() => setResponding(false)} onCreated={onCreated} /> }</> }</div> }</DataState>;
};


const ResponseForm = ( { scenario, forecast, onClose, onCreated }: { scenario: IScenario; forecast: IForecastSucceeded; onClose: () => void; onCreated: ( id: string ) => void } ) => {
    const { company } = useCompany();
    const request = useRequest();
    const eligible = forecast.result.effective_occurrences.filter(( item ) => item.response_action_eligible && item.in_horizon && BigInt(item.remaining_amount.amount_minor) > BigInt(0));


    const save = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const occurrence = eligible.find(( item ) => item.effective_occurrence_id === form.get("target"));
            const amount = parseMoney(textField(form, "amount", "Сумма"));
            const date = dateField(form, "date", "Новая дата");

            if (!occurrence) { return invalid("Выберите доступную часть расхода.", "target"); }
            if (date <= occurrence.date) { return invalid("Новая дата должна быть позже текущей даты расхода.", "date"); }
            if (BigInt(amount) > BigInt(occurrence.remaining_amount.amount_minor)) { return invalid("Сумма превышает доступную часть расхода.", "amount"); }

            return scenariosService.createScenario({ company_id: company.id, idempotency_key: key, data: { name: textField(form, "name", "Название"), comparison_parent_scenario_id: scenario.id, response_action: { type: "reschedule_payment_part", target: { effective_occurrence_id: occurrence.effective_occurrence_id }, amount: { amount_minor: amount, currency: "RUB" }, new_date: date, feasibility_condition: textField(form, "condition", "Условие выполнения", 1000) } } }, signal);
        }, "Ответный вариант принят на расчёт", formIdentity(form));

        if (result.ok) { onCreated(result.data.id); onClose(); }
    };


    return <Modal title="Перенос части расхода" busy={request.busy} onClose={onClose}><form className={styles.form} noValidate onSubmit={save}><Notice>Проверяем действие поверх выбранного стресса. Это расчётный вариант: реальные платежи и календарь не изменятся.</Notice><fieldset disabled={request.busy}><Field name="name" label="Название варианта" defaultValue={`${scenario.name.slice(0, 150)} + перенос`} /><Select disabled={request.busy} name="target" label="Какую часть расхода перенести"><option value="">Выберите расход</option>{ eligible.map(( item ) => <option key={item.effective_occurrence_id} value={item.effective_occurrence_id}>{ formatDate(item.date) } · { formatMoney(item.remaining_amount.amount_minor) }</option>) }</Select><Field name="amount" label="Сумма переноса, ₽" inputMode="decimal" placeholder="0,00" /><Field name="date" label="Новая дата" type="date" defaultValue={addDays(forecast.input.end_date, 1)} /><TextArea name="condition" label="При каком условии перенос возможен" placeholder="Например, поставщик согласует отсрочку до указанной даты" /></fieldset><p className={styles.muted}>При переносе за пределы горизонта расход не попадёт в остаток этого периода, но обязательство сохранится.</p><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Рассчитать ответный вариант</ActionButton></form></Modal>;
};


export default Scenarios;
