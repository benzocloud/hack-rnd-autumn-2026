"use client";

import axios from "axios";
import { useEffect, useRef, useState } from "react";
import { useDispatch } from "react-redux";

import { companyPath, formatDate } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar } from "@/helpers/snackbar";
import type { ForecastPeriod, IDashboardData } from "@/interfaces";
import { dashboardService } from "@/service";
import { setSelectedCompany } from "@/store/companySlice";
import type { AppDispatch } from "@/store/store";
import { LinkButton, Notice } from "@/widgets/FinanceUI/FinanceUI";
import { AuthButton, BalanceChart, LoadingText, Spinner } from "@/widgets";

import styles from "./style.module.scss";


const periods: ForecastPeriod[] = [ 30, 60, 90 ];
const dateFormatter = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", timeZone: "UTC" });


const Dashboard = ( { companyId }: { companyId?: string } ) => {
    const [ period, setPeriod ] = useState<ForecastPeriod>(30);
    const [ attempt, setAttempt ] = useState(0);
    const [ loading, setLoading ] = useState(true);
    const [ failed, setFailed ] = useState(false);
    const [ data, setData ] = useState<IDashboardData | null>(null);
    const cache = useRef<Partial<Record<ForecastPeriod, IDashboardData>>>({});
    const dispatch = useDispatch<AppDispatch>();


    useEffect(() => {
        const controller = new AbortController();
        const request = cache.current[period] ? Promise.resolve(cache.current[period]) : dashboardService.getDashboardData(period, controller.signal, companyId || (() => { try { return localStorage.getItem("saldo-company-id") || undefined; } catch { return undefined; } })());

        request.then(( response ) => {
            if (controller.signal.aborted) {
                return;
            }

            cache.current[period] = response;
            setData(response);
            dispatch(setSelectedCompany(response.company));
            setLoading(false);
        }).catch(( error: unknown ) => {
            if (axios.isCancel(error) || controller.signal.aborted) {
                return;
            }

            if (!axios.isAxiosError(error)) {
                showErrorSnackbar(getErrorMessage(error));
            }

            setFailed(true);
            setLoading(false);
        });

        return () => controller.abort();
    }, [ period, attempt, dispatch, companyId ]);


    const changePeriod = ( next: ForecastPeriod ) => {
        if (loading || period === next) {
            return;
        }

        setLoading(true);
        setFailed(false);
        setPeriod(next);
    };


    const retry = () => {
        if (loading) {
            return;
        }

        setLoading(true);
        setFailed(false);
        setAttempt(( current ) => current + 1);
    };


    const gap = !loading ? data?.forecast?.result.metrics.first_gap : null;
    const ready = !loading && !!data?.forecast && !failed;
    const status = loading ? <LoadingText>Загружаем прогноз</LoadingText> : failed ? "Не удалось загрузить данные" : ready ? gap ? "ОЖИДАЕТСЯ КАССОВЫЙ РАЗРЫВ" : "КАССОВЫЙ РАЗРЫВ НЕ ПРОГНОЗИРУЕТСЯ" : "НЕДОСТАТОЧНО ДАННЫХ";
    const warning = loading ? <LoadingText>Проверяем данные</LoadingText> : failed ? "Попробуйте повторить загрузку" : gap ? `${dateFormatter.format(new Date(`${gap.date}T00:00:00Z`)).toLocaleUpperCase("ru-RU")} ПРОГНОЗИРУЕТСЯ КАССОВЫЙ РАЗРЫВ` : ready ? data?.forecast?.readiness.limitations[0] || "На выбранный период предупреждений нет" : data?.company ? "Укажите датированный остаток на счёте для построения прогноза" : "Для прогноза нужна компания с данными счёта";


    return (
        <>
        { !companyId && <nav className={styles.navigation} aria-label="Рабочее пространство"><LinkButton href="/workspace" secondary icon="building">Все компании</LinkButton>{ data?.company && <LinkButton href={companyPath(data.company.id)} secondary>Разделы компании</LinkButton> }</nav> }
        <main className={styles.dashboard} aria-label="Финансовый дашборд">
            <h1 className={styles.hidden}>Финансовый дашборд</h1>

            <div className={styles.summary} aria-live="polite">
                <section className={styles.card}>
                    <h2>{ data?.account?.balance_as_of ? `Остаток на ${formatDate(data.account.balance_as_of)}:` : "Остаток на счёте:" }</h2>
                    <p>{ data?.account?.balance ? formatMoney(data.account.balance.amount_minor) : loading ? <LoadingText>Загружаем</LoadingText> : "Не указан" }</p>
                </section>

                <section className={styles.card}>
                    <h2>Ваша ситуация:</h2>
                    <p className={ready ? gap ? styles.danger : styles.success : undefined}>{ status }</p>
                </section>

                <section className={styles.card}>
                    <h2>Предупреждения:</h2>
                    <p className={gap ? styles.danger : undefined}>{ warning }</p>
                </section>
            </div>

            <section className={styles.chartCard} aria-label={`Прогноз на ${period} дней`} aria-busy={loading}>
                <div className={styles.chartArea}>
                    { ready && data?.forecast ? (
                        <BalanceChart key={data.forecast.id} points={data.forecast.result.daily_points} />
                    ) : (
                        <div className={styles.placeholder}>
                            { loading ? <Spinner label="Загружаем график" /> : <p>{ failed ? "Не удалось построить график" : "Данных для графика пока нет" }</p> }
                        </div>
                    ) }
                </div>

                <div className={styles.periods} role="group" aria-label="Период прогноза" aria-busy={loading}>
                    <div className={`${styles.periodOptions} ${loading ? styles.contentHidden : ""}`} aria-hidden={loading || undefined}>
                        { periods.map(( item ) => (
                            <button className={styles.period} type="button" key={item} aria-pressed={period === item} disabled={loading} onClick={() => changePeriod(item)}>{ item } дней</button>
                        )) }
                    </div>
                    { loading && <span className={styles.periodSpinner}><Spinner label="Загружаем период прогноза" /></span> }
                </div>

                { failed && <div className={styles.retry}><AuthButton type="button" loading={loading} onClick={retry}>Повторить загрузку</AuthButton></div> }
            </section>
            <div className={styles.details}>
                { data?.forecast?.is_stale && <Notice tone="warning">После расчёта данные изменились. Этот прогноз отражает сохранённый снимок.</Notice> }
                { data?.company ? <div className={styles.links}><LinkButton href={companyPath(data.company.id, "settings")} secondary>Остаток и реквизиты</LinkButton><LinkButton href={companyPath(data.company.id, "calendar")} secondary>Ближайшие платежи</LinkButton>{ data.forecast && <LinkButton href={companyPath(data.company.id, `forecasts/${data.forecast.id}`)}>Подробнее о прогнозе</LinkButton> }</div> : !loading && <LinkButton href="/workspace">Добавить компанию</LinkButton> }
                { data?.forecast && <p className={styles.note}>Прогноз по данным на { formatDate(data.forecast.input.data_as_of) }. Вероятность отрицательного остатка: { data.forecast.result.metrics.gap_probability.value === null ? "не оценена" : `${Math.round(data.forecast.result.metrics.gap_probability.value * 100)}%` }.</p> }
            </div>
        </main>
        </>
    );
};


export default Dashboard;
