"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { companyPath, dateField, formatDate, formIdentity, invalid } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import useJobResource from "@/hooks/useJobResource";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IForecastSucceeded } from "@/interfaces";
import { forecastsService } from "@/service";
import { BalanceChart } from "@/widgets";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, JobState, LinkButton, Modal, Notice, PageHeading, Panel, Select } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


const ForecastMetrics = ( { forecast }: { forecast: IForecastSucceeded } ) => {
    const metrics = forecast.result.metrics;

    return <div className={styles.grid}><Panel title="Первый отрицательный остаток"><p className={styles.stat}>{ metrics.first_gap ? formatDate(metrics.first_gap.date) : forecast.readiness.status === "insufficient" ? "Недостаточно данных" : "Не прогнозируется" }</p>{ metrics.first_gap && <p>Дефицит { formatMoney(metrics.first_gap.shortfall.amount_minor) }</p> }</Panel><Panel title="Минимальный остаток"><p className={styles.stat}>{ formatMoney(metrics.minimum_balance.amount_minor) }</p><p className={styles.muted}>На конец дня в выбранном периоде</p></Panel><Panel title="Вероятность отрицательного остатка"><p className={styles.stat}>{ metrics.gap_probability.value === null ? "Не оценена" : `${Math.round(metrics.gap_probability.value * 100)}%` }</p><p className={styles.muted}>{ metrics.gap_probability.value === null ? "Модель не предоставила достоверную оценку вероятности." : "Хотя бы в один день выбранного периода" }</p></Panel></div>;
};


const ForecastDetail = () => {
    const { forecastId } = useParams<{ forecastId: string }>();
    const { company } = useCompany();
    const [ tab, setTab ] = useState<"forecast" | "actual">("forecast");
    const resource = useJobResource(useCallback(( signal: AbortSignal ) => forecastsService.getForecast({ company_id: company.id, forecast_id: forecastId }, signal), [ company.id, forecastId ]));
    const forecast = resource.data;


    return <main className={styles.page}><PageHeading title="Сохранённый прогноз" description={forecast ? `${formatDate(forecast.input.start_date)} — ${formatDate(forecast.input.end_date)} · данные на ${formatDate(forecast.input.data_as_of)}` : "Расчёт денежных потоков на выбранный период."}><CreateForecastButton /><LinkButton href={companyPath(company.id, "scenarios")} secondary icon="adjustments">Сценарии</LinkButton></PageHeading><DataState loading={resource.loading && !forecast} error={resource.error} onRetry={resource.reload}>{ forecast && <div className={styles.stack}>{ forecast.is_stale && <Notice tone="warning">Данные компании изменились после этого расчёта. Результат сохранён для сравнения; постройте новый прогноз для актуального состояния.</Notice> }{ forecast.status === "failed" ? <Panel><EmptyState title="Расчёт не завершился" icon="alert-triangle" action={<CreateForecastButton />}>{ forecast.error.message || "Попробуйте создать новый прогноз." }</EmptyState></Panel> : forecast.status !== "succeeded" ? <Panel><JobState label="Строим прогноз" /></Panel> : <><div className={styles.row}><ActionButton variant="secondary" aria-pressed={tab === "forecast"} onClick={() => setTab("forecast")}>Прогноз</ActionButton><ActionButton variant="secondary" aria-pressed={tab === "actual"} onClick={() => setTab("actual")}>План-факт</ActionButton><Badge tone={forecast.readiness.status === "ready" ? "success" : "warning"}>{ { ready: "Данные готовы", limited: "Есть ограничения", insufficient: "Недостаточно данных" }[forecast.readiness.status] }</Badge></div>{ tab === "actual" ? <PlanFact forecast={forecast} /> : <><ForecastMetrics forecast={forecast} /><Panel title="Остаток по дням"><div className={styles.chart}><BalanceChart points={forecast.result.daily_points} /></div><p className={styles.muted}>Модельный остаток ниже нуля — индикатор риска, а не подтверждение невозможности платежа. Условия овердрафта в расчёте не учитываются.</p></Panel><Panel title="Качество данных и допущения"><p>История: { forecast.readiness.history_days } дней, наблюдения: { forecast.readiness.observed_days } дней.</p>{ [ ...forecast.readiness.limitations, ...forecast.result.assumptions ].map(( item, index ) => <p key={index}>{ item }</p>) }{ forecast.readiness.quality_issues.map(( issue, index ) => <Notice key={index} tone={issue.severity === "blocking" ? "danger" : "warning"}>{ issue.message }</Notice>) }<p className={styles.muted}>Версия данных: { forecast.input.data_version }. Модель: { forecast.input.model_context.model_name } { forecast.input.model_context.model_version }.</p></Panel><Panel title="Рекомендации">{ forecast.result.recommendations.length ? forecast.result.recommendations.map(( item ) => <article className={styles.recommendation} key={item.id}><h3>{ item.title }</h3><p>{ item.explanation }</p><Notice>Условие выполнения: { item.feasibility_condition }</Notice>{ item.evidence.length > 0 && <details><summary>На чём основана рекомендация</summary>{ item.evidence.map(( evidence, index ) => <p key={index}>{ evidence.description }</p>) }</details> }</article>) : <p>Дополнительных рекомендаций для этого расчёта нет. Вы можете самостоятельно проверить варианты в разделе «Сценарии».</p> }</Panel></> }</> }</div> }</DataState></main>;
};


const CreateForecastButton = () => {
    const { company, account } = useCompany();
    const router = useRouter();
    const [ open, setOpen ] = useState(false);
    const request = useRequest();


    const create = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const cutoff = dateField(form, "cutoff_date", "Дата среза");
            const horizon = Number(form.get("horizon"));

            if (![ 30, 60, 90 ].includes(horizon)) { invalid("Выберите период прогноза.", "horizon"); }
            if (!account.balance || cutoff !== account.balance_as_of) { invalid("Для выбранной даты нужен подтверждённый остаток. Укажите его в разделе «Компания».", "cutoff_date"); }

            return forecastsService.createForecast({ company_id: company.id, idempotency_key: key, data: { cutoff_date: cutoff, horizon_days: horizon } }, signal);
        }, "Расчёт принят", formIdentity(form));

        if (result.ok) { router.push(companyPath(company.id, `forecasts/${result.data.id}`)); setOpen(false); }
    };


    return <><ActionButton icon="chart-line" onClick={() => setOpen(true)}>Новый прогноз</ActionButton>{ open && <Modal title="Построить прогноз" busy={request.busy} onClose={() => setOpen(false)}><form noValidate className={styles.form} onSubmit={create}><Notice>Расчёт сохранит текущие операции и платежи в отдельном снимке. Уже созданные прогнозы останутся доступными.</Notice><fieldset disabled={request.busy}><Field name="cutoff_date" label="Дата среза" type="date" defaultValue={account.balance_as_of || ""} hint="Нужен остаток на конец этого дня." /><Select disabled={request.busy} name="horizon" label="На сколько дней"><option value="30">30 дней</option><option value="60">60 дней</option><option value="90">90 дней</option></Select></fieldset>{ !account.balance && <LinkButton href={companyPath(company.id, "settings")} secondary>Указать остаток</LinkButton> }<FormError message={request.error} /><ActionButton type="submit" loading={request.busy} disabled={!account.balance}>Построить прогноз</ActionButton></form></Modal> }</>;
};


const PlanFact = ( { forecast }: { forecast: IForecastSucceeded } ) => {
    const { company } = useCompany();
    const [ period, setPeriod ] = useState<{ from?: string; to?: string }>({});
    const request = useRequest();
    const resource = useResource(useCallback(( signal: AbortSignal ) => forecastsService.getPlanFact({ company_id: company.id, forecast_id: forecast.id, ...period }, signal), [ company.id, forecast.id, period ]));


    const filter = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);

        await request.run(async () => {
            const from = dateField(form, "from", "Начало периода");
            const to = dateField(form, "to", "Конец периода");

            if (from > to || from < forecast.input.start_date || to > forecast.input.end_date) { invalid("Выберите период внутри горизонта прогноза.", "from"); }

            setPeriod({ from, to });
        });
    };


    return <Panel title="План-факт"><div className={styles.stack}><form noValidate onSubmit={filter} className={styles.filters}><Field name="from" label="Начало периода" type="date" defaultValue={forecast.input.start_date} /><Field name="to" label="Конец периода" type="date" defaultValue={forecast.input.end_date} /><ActionButton type="submit" variant="secondary" loading={request.busy}>Показать</ActionButton></form><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && <><Notice tone={resource.data.coverage.status === "complete" ? "success" : "warning"}>{ resource.data.coverage.status === "complete" ? "Фактические данные покрывают весь период." : resource.data.coverage.status === "partial" ? "Факт покрывает период частично. Пустые значения не означают нулевое движение." : "Фактических данных за этот период пока нет." }</Notice>{ resource.data.coverage.warnings.map(( warning, index ) => <p key={index}>{ warning }</p>) }<div className={styles.scroll}><table className={styles.table}><caption className={styles.muted}>Остаток на конец дня. «Нет данных» означает отсутствие подтверждённого факта.</caption><thead><tr><th>Дата</th><th>План</th><th>Факт</th><th>Отклонение</th><th>Покрытие</th></tr></thead><tbody>{ resource.data.daily_points.map(( item ) => <tr key={item.date}><td>{ formatDate(item.date) }</td><td className={styles.money}>{ formatMoney(item.planned_balance.amount_minor) }</td><td className={styles.money}>{ item.actual_balance ? formatMoney(item.actual_balance.amount_minor) : "Нет данных" }</td><td className={styles.money}>{ item.balance_delta ? formatMoney(item.balance_delta.amount_minor) : "Не рассчитано" }</td><td>{ { complete: "Полное", partial: "Частичное", unavailable: "Нет данных" }[item.coverage] }</td></tr>) }</tbody></table></div>{ resource.data.coverage.missing_dates.length > 0 && <details><summary>Даты без факта ({ resource.data.coverage.missing_dates.length })</summary><p>{ resource.data.coverage.missing_dates.map(( value ) => formatDate(value)).join(", ") }</p></details> }<p>Средняя абсолютная ошибка: { resource.data.mean_absolute_balance_error ? formatMoney(resource.data.mean_absolute_balance_error.amount_minor) : "Не рассчитана" }.</p></> }</DataState></div></Panel>;
};


export { CreateForecastButton, ForecastMetrics };
export default ForecastDetail;
