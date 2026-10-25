"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { companyPath, dateField, formatDate, formIdentity, invalid, sourceName } from "@/helpers/finance";
import { categorySourceLabel, categorySourceTone, directionLabel, hasForecastAnalysis, hypothesisScheduleLabel } from "@/helpers/forecastAnalysis";
import { forecastQualityDescription } from "@/helpers/forecastQuality";
import { getErrorDetailMessage } from "@/helpers/getErrorMessage";
import formatMoney from "@/helpers/formatMoney";
import useJobResource from "@/hooks/useJobResource";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IForecastSucceeded, ITransaction } from "@/interfaces";
import { forecastsService, ledgerService } from "@/service";
import { BalanceChart } from "@/widgets";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, JobState, LinkButton, Modal, Notice, PageHeading, Panel, Select } from "@/widgets/FinanceUI/FinanceUI";

import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


const ForecastMetrics = ( { forecast }: { forecast: IForecastSucceeded } ) => {
    const metrics = forecast.result.metrics;

    return <div className={styles.grid}><Panel title="Первый отрицательный остаток"><p className={styles.stat}>{ metrics.first_gap ? formatDate(metrics.first_gap.date) : forecast.readiness.status === "insufficient" ? "Недостаточно данных" : "Не прогнозируется" }</p>{ metrics.first_gap && <p>Дефицит { formatMoney(metrics.first_gap.shortfall.amount_minor) }</p> }</Panel><Panel title="Минимальный остаток"><p className={styles.stat}>{ formatMoney(metrics.minimum_balance.amount_minor) }</p><p className={styles.muted}>На конец дня в выбранном периоде</p></Panel><Panel title="Вероятность отрицательного остатка"><p className={styles.stat}>{ metrics.gap_probability.value === null ? "Не оценена" : `${Math.round(metrics.gap_probability.value * 100)}%` }</p><p className={styles.muted}>{ metrics.gap_probability.value === null ? "Модель не предоставила достоверную оценку вероятности." : "Хотя бы в один день выбранного периода" }</p></Panel></div>;
};


const operationLabel = ( transactionId: string, labels: Map<string, ITransaction> ): string => {
    const transaction = labels.get(transactionId);

    if (!transaction) {
        return transactionId;
    }

    return `${transaction.counterparty_name} · ${transaction.payment_purpose}`;
};


const ForecastAnalysisPanel = ( { forecast }: { forecast: IForecastSucceeded } ) => {
    const { company } = useCompany();
    const analysis = forecast.result.analysis ?? null;
    const labels = useResource(useCallback(async ( signal: AbortSignal ) => {
        if (!hasForecastAnalysis(analysis)) {
            return new Map<string, ITransaction>();
        }

        const page = await ledgerService.listTransactions({ company_id: company.id, from: analysis.window_from, to: analysis.window_through, limit: 100 }, signal);
        const map = new Map(page.items.map(( item ) => [ item.id, item ]));

        return map;
    }, [ analysis, company.id ]));

    if (!hasForecastAnalysis(analysis)) {
        return <Panel title="Анализ категорий и регулярности"><EmptyState title="Анализ недоступен" icon="info-circle">Для этого сохранённого прогноза анализ категорий и гипотез регулярности не выполнялся. Это не означает, что категории операций неизвестны.</EmptyState></Panel>;
    }

    const transactionLabels = labels.data || new Map<string, ITransaction>();

    return <Panel title="Анализ категорий и регулярности"><div className={styles.stack}><Notice tone={forecast.is_stale ? "warning" : "neutral"}>Снимок анализа для окна { formatDate(analysis.window_from) } — { formatDate(analysis.window_through) }. Версия { analysis.analysis_version }. Результат относится к этому прогнозу{ forecast.is_stale ? " и может не совпадать с текущими операциями" : "" }; это не оценка точности и не обязательство.</Notice><details open><summary>Категории операций ({ analysis.classified_operations.length })</summary>{ analysis.classified_operations.length ? <div className={styles.scroll}><table className={styles.table}><thead><tr><th>Операция</th><th>Категория</th><th>Источник</th><th>Версия</th></tr></thead><tbody>{ analysis.classified_operations.map(( item ) => <tr key={`${item.transaction_id}:${item.version}`}><td><strong>{ operationLabel(item.transaction_id, transactionLabels) }</strong>{ transactionLabels.has(item.transaction_id) && <div className={styles.muted}>{ item.transaction_id }</div> }</td><td>{ item.category_code }</td><td><Badge tone={categorySourceTone(item.category_source)}>{ categorySourceLabel(item.category_source) }</Badge>{ item.category_source === "rules" && item.category_rule_id && <div className={styles.muted}>{ item.category_rule_id }</div> }</td><td>{ item.version }</td></tr>) }</tbody></table></div> : <p className={styles.muted}>В окне анализа нет классифицированных операций.</p> }</details><div><h3 className={styles.sectionTitle}>Гипотезы регулярности</h3><p className={styles.muted}>Гипотеза — предположение о повторяющемся движении, а не платёж в календаре и не вероятность риска.</p>{ analysis.recurring_hypotheses.length ? analysis.recurring_hypotheses.map(( hypothesis ) => <article className={styles.item} key={hypothesis.pattern_id}><div><h3>{ directionLabel(hypothesis.direction) }: { hypothesis.category_code }</h3><p>Ожидаемая сумма { formatMoney(hypothesis.expected_amount_minor) }. Расписание: { hypothesisScheduleLabel(hypothesis) }.</p><p className={styles.muted}>Подтверждений: { hypothesis.confirmations } из { hypothesis.expected_periods }. Период свидетельств: { formatDate(hypothesis.first_evidence_date) } — { formatDate(hypothesis.last_evidence_date) }.</p></div><Badge tone="warning">Гипотеза</Badge><details className={styles.evidence}><summary>Подтверждающие операции ({ hypothesis.historical_operations.length })</summary><ul className={styles.evidenceList}>{ hypothesis.historical_operations.map(( ref ) => <li key={`${ref.transaction_id}:${ref.version}`}><strong>{ operationLabel(ref.transaction_id, transactionLabels) }</strong><span className={styles.muted}> · версия { ref.version }{ !transactionLabels.has(ref.transaction_id) ? ` · ${ref.transaction_id}` : "" }</span></li>) }</ul></details></article>) : <p className={styles.muted}>Устойчивых гипотез регулярности для этого окна нет.</p> }</div></div></Panel>;
};


const ForecastDetail = () => {
    const { forecastId } = useParams<{ forecastId: string }>();
    const { company } = useCompany();
    const [ tab, setTab ] = useState<"forecast" | "actual">("forecast");
    const resource = useJobResource(useCallback(( signal: AbortSignal ) => forecastsService.getForecast({ company_id: company.id, forecast_id: forecastId }, signal), [ company.id, forecastId ]));
    const forecast = resource.data;


    return <main className={styles.page}><PageHeading title="Сохранённый прогноз" description={forecast ? `${formatDate(forecast.input.start_date)} — ${formatDate(forecast.input.end_date)} · данные на ${formatDate(forecast.input.data_as_of)}` : "Расчёт денежных потоков на выбранный период."}><CreateForecastButton /><LinkButton href={companyPath(company.id, "scenarios")} secondary icon="adjustments">Сценарии</LinkButton></PageHeading><DataState loading={resource.loading && !forecast} error={resource.error} onRetry={resource.reload}>{ forecast && <div className={styles.stack}>{ forecast.is_stale && <Notice tone="warning">Данные компании изменились после этого расчёта. Результат сохранён для сравнения; постройте новый прогноз для актуального состояния.</Notice> }{ forecast.status === "failed" ? <Panel><EmptyState title="Расчёт не завершился" icon="alert-triangle" action={<CreateForecastButton />}>{ getErrorDetailMessage(forecast.error) }</EmptyState></Panel> : forecast.status !== "succeeded" ? <Panel>{ [ "queued", "running" ].includes(forecast.status) ? <JobState label={forecast.status === "queued" ? "Прогноз в очереди" : "Строим прогноз"} /> : <Notice tone="warning">Расчёт остановлен. Сохранённый результат недоступен.</Notice> }</Panel> : <>{ forecast.input.history_assessment && <Notice tone="warning">Ограниченная оценка по CSV: часть истории может быть неизвестна. Учитывайте предупреждения о качестве данных.</Notice> }<div className={styles.row}><ActionButton variant="secondary" aria-pressed={tab === "forecast"} onClick={() => setTab("forecast")}>Прогноз</ActionButton><ActionButton variant="secondary" aria-pressed={tab === "actual"} onClick={() => setTab("actual")}>План-факт</ActionButton><Badge tone={forecast.readiness.status === "ready" ? "success" : "warning"}>{ { ready: "Данные готовы", limited: "Есть ограничения", insufficient: "Недостаточно данных" }[forecast.readiness.status] }</Badge></div>{ tab === "actual" ? <PlanFact forecast={forecast} /> : <><ForecastMetrics forecast={forecast} /><Panel title="Остаток по дням"><div className={styles.chart}><BalanceChart points={forecast.result.daily_points} /></div><details><summary>Точные суммы по дням</summary><div className={styles.scroll}><table className={styles.table}><thead><tr><th>Дата</th><th>Поступления</th><th>Расходы</th><th>Прогноз остатка</th></tr></thead><tbody>{ forecast.result.daily_points.map(( point ) => <tr key={point.date}><td>{ formatDate(point.date) }</td><td>{ formatMoney(point.inflow.amount_minor) }</td><td>{ formatMoney(point.outflow.amount_minor) }</td><td>{ formatMoney(point.balance.amount_minor) }</td></tr>) }</tbody></table></div></details><p className={styles.muted}>Модельный остаток ниже нуля — индикатор риска, а не подтверждение невозможности платежа. Условия овердрафта в расчёте не учитываются.</p></Panel><Panel title="Календарь сохранённого расчёта"><Notice tone={forecast.input.calendar_knowledge_mode === "current" || forecast.input.calendar_knowledge_mode === "historical_as_of_certified" ? "neutral" : "warning"}>{ { current: "Календарь известен на момент расчёта.", historical_as_of_certified: "Историческая доступность календаря подтверждена сервером.", synthetic_scenario: "Синтетический сценарий календаря. Это не подтверждённая историческая доступность обязательств.", unavailable: "Сведения о календаре недоступны. Это не означает отсутствия обязательств или риска." }[forecast.input.calendar_knowledge_mode || "unavailable"] }{ forecast.input.calendar_as_of_date && ` Срез календаря: ${formatDate(forecast.input.calendar_as_of_date)}.` }</Notice>{ forecast.result.components.some(( item ) => item.kind === "known_payment") ? <div className={styles.scroll}><table className={styles.table}><thead><tr><th>Платёж из снимка</th><th>Дата</th><th>Сумма</th><th>Осталось</th><th>Источник</th></tr></thead><tbody>{ forecast.result.components.filter(( item ) => item.kind === "known_payment").map(( item ) => <tr key={item.id}><td>{ item.payment_id || item.occurrence_id }</td><td>{ formatDate(item.date) }</td><td>{ item.direction === "inflow" ? "+" : "−" }{ formatMoney(item.amount.amount_minor) }</td><td>{ formatMoney(item.remaining_amount.amount_minor) }</td><td>{ item.source ? sourceName(item.source.kind) : "Не указан" }</td></tr>) }</tbody></table></div> : <p>Результат не содержит отдельных компонентов календаря. Это не подтверждает отсутствия обязательств.</p> }<p className={styles.muted}>Показан сохранённый снимок. Изменения текущих платежей появятся только в новом прогнозе.</p></Panel><ForecastAnalysisPanel forecast={forecast} /><Panel title="Качество данных и допущения"><p>Окно расчёта: { forecast.readiness.history_days } дней. Даты с загруженными операциями: { forecast.readiness.observed_days }.</p>{ [ ...forecast.readiness.limitations, ...forecast.result.assumptions ].map(( item, index ) => <p key={index}>{ forecastQualityDescription(item) }</p>) }{ forecast.readiness.quality_issues.map(( issue, index ) => <Notice key={index} tone={issue.severity === "blocking" ? "danger" : "warning"}>{ issue.message }</Notice>) }<p className={styles.muted}>Версия данных: { forecast.input.data_version }. Модель: { forecast.input.model_context.model_name } { forecast.input.model_context.model_version }.</p></Panel><Panel title="Рекомендации">{ forecast.result.recommendations.length ? forecast.result.recommendations.map(( item ) => <article className={styles.recommendation} key={item.id}><h3>{ item.title }</h3><p>{ item.explanation }</p><Notice>Условие выполнения: { item.feasibility_condition }</Notice>{ item.evidence.length > 0 && <details><summary>На чём основана рекомендация</summary>{ item.evidence.map(( evidence, index ) => <p key={index}>{ evidence.description }</p>) }</details> }</article>) : <p>Дополнительных рекомендаций для этого расчёта нет. Вы можете самостоятельно проверить варианты в разделе «Сценарии».</p> }</Panel></> }</> }</div> }</DataState></main>;
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

            if (horizon !== 30) { invalid("Выберите период прогноза.", "horizon"); }
            if (!account.balance || cutoff !== account.balance_as_of) { invalid("Для выбранной даты нужен подтверждённый остаток. Укажите его в разделе «Компания».", "cutoff_date"); }

            return forecastsService.createForecast({ company_id: company.id, idempotency_key: key, data: { cutoff_date: cutoff, horizon_days: horizon } }, signal);
        }, "Расчёт принят", formIdentity(form));

        if (result.ok) { router.push(companyPath(company.id, `forecasts/${result.data.id}`)); setOpen(false); }
    };


    return <><ActionButton icon="chart-line" onClick={() => setOpen(true)}>Рассчитать новый прогноз</ActionButton>{ open && <Modal title="Построить прогноз" busy={request.busy} onClose={() => setOpen(false)}><ValidatedForm request={request} noValidate className={styles.form} onSubmit={create}><Notice>Расчёт сохранит текущие операции и платежи в отдельном снимке. Уже созданные прогнозы останутся доступными.</Notice><fieldset disabled={request.busy}><Field required data-maximum="200" name="cutoff_date" label="Дата среза" type="date" defaultValue={account.balance_as_of || ""} hint="Нужен остаток на конец этого дня." /><Select disabled={request.busy} name="horizon" label="На сколько дней"><option value="30">30 дней</option></Select></fieldset>{ !account.balance && <LinkButton href={companyPath(company.id, "settings")} secondary>Указать остаток</LinkButton> }<FormError message={request.error} /><ActionButton type="submit" loading={request.busy} disabled={!account.balance}>Построить прогноз</ActionButton></ValidatedForm></Modal> }</>;
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


    return <Panel title="План-факт"><div className={styles.stack}><ValidatedForm request={request} noValidate onSubmit={filter} className={styles.filters}><Field data-maximum="200" name="from" label="Начало периода" type="date" defaultValue={forecast.input.start_date} /><Field data-maximum="200" name="to" label="Конец периода" type="date" defaultValue={forecast.input.end_date} /><ActionButton type="submit" variant="secondary" loading={request.busy}>Показать</ActionButton></ValidatedForm><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && <><Notice tone={resource.data.coverage.status === "complete" ? "success" : "warning"}>{ resource.data.coverage.status === "complete" ? "Фактические данные покрывают весь период." : resource.data.coverage.status === "partial" ? "Факт покрывает период частично. Пустые значения не означают нулевое движение." : "Фактических данных за этот период пока нет." }</Notice>{ resource.data.coverage.warnings.map(( warning, index ) => <p key={index}>{ warning }</p>) }<div className={styles.scroll}><table className={styles.table}><caption className={styles.muted}>Остаток на конец дня. «Нет данных» означает отсутствие подтверждённого факта.</caption><thead><tr><th>Дата</th><th>План</th><th>Факт</th><th>Отклонение</th><th>Покрытие</th></tr></thead><tbody>{ resource.data.daily_points.map(( item ) => <tr key={item.date}><td>{ formatDate(item.date) }</td><td className={styles.money}>{ formatMoney(item.planned_balance.amount_minor) }</td><td className={styles.money}>{ item.actual_balance ? formatMoney(item.actual_balance.amount_minor) : "Нет данных" }</td><td className={styles.money}>{ item.balance_delta ? formatMoney(item.balance_delta.amount_minor) : "Не рассчитано" }</td><td>{ { complete: "Полное", partial: "Частичное", unavailable: "Нет данных" }[item.coverage] }</td></tr>) }</tbody></table></div>{ resource.data.coverage.missing_dates.length > 0 && <details><summary>Даты без факта ({ resource.data.coverage.missing_dates.length })</summary><p>{ resource.data.coverage.missing_dates.map(( value ) => formatDate(value)).join(", ") }</p></details> }<p>Средняя абсолютная ошибка: { resource.data.mean_absolute_balance_error ? formatMoney(resource.data.mean_absolute_balance_error.amount_minor) : "Не рассчитана" }.</p></> }</DataState></div></Panel>;
};


export { CreateForecastButton, ForecastMetrics, ForecastAnalysisPanel };
export default ForecastDetail;
