"use client";

import { useState } from "react";

import { makeDemoForecast } from "@/helpers/demoForecast";
import formatMoney from "@/helpers/formatMoney";
import BalanceChart from "@/widgets/BalanceChart/BalanceChart";
import Icon from "@/widgets/Icon/Icon";

import styles from "./style.module.scss";


const dateFormatter = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", timeZone: "UTC" });


const ForecastDemo = () => {
    const [ period, setPeriod ] = useState(30);
    const points = makeDemoForecast(period);
    const gap = points.find(( point ) => BigInt(point.balance.amount_minor) < BigInt(0));
    const minimum = points.reduce(( value, point ) => BigInt(point.balance.amount_minor) < value ? BigInt(point.balance.amount_minor) : value, BigInt(points[0].balance.amount_minor));

    return (
        <div className={styles.forecastCard} id="demo" aria-label="Демонстрация прогноза">
            <div className={styles.windowBar}><span /><span /><span /><span className={styles.windowLabel}>saldo / обзор финансов</span><span className={styles.demoBadge}>Демо</span></div>
            <div className={styles.forecastBody}>
                <div className={styles.forecastHeading}><div><span className={styles.muted}>Ваш бизнес · пример</span><h2>Деньги под контролем</h2></div><span className={styles.companyIcon}><Icon name="building" size={22} /></span></div>
                <div className={styles.summaryRow}><div><span className={styles.muted}>Остаток на 24 октября</span><strong>680 000 <span>₽</span></strong></div><span className={styles.currencyBadge}>RUB</span></div>
                <div className={styles.chartHeader}><h3>Прогноз остатка</h3><div className={styles.periods} role="group" aria-label="Период прогноза">{ [ 30, 60, 90 ].map(( value ) => <button type="button" key={value} aria-pressed={period === value} onClick={() => setPeriod(value)}>{ value } дн.</button>) }</div></div>
                <BalanceChart points={points} />
                <div className={styles.chartLegend}><span><i />Остаток на счёте, ₽</span><span>24 окт. — { dateFormatter.format(new Date(`${points[points.length - 1].date}T00:00:00Z`)) }</span></div>
                <div className={styles.gapNotice} aria-live="polite"><Icon name="alert-triangle" size={20} /><div><strong>{ gap ? `${dateFormatter.format(new Date(`${gap.date}T00:00:00Z`))} может не хватить денег` : "На выбранный период разрыва нет" }</strong><span>Минимальный остаток: { formatMoney(minimum.toString()) }</span></div><span className={styles.noticeDot} /></div>
                <p className={styles.demoDisclaimer}>Демонстрационные данные. Прогноз не является гарантией.</p>
            </div>
        </div>
    );
};


export default ForecastDemo;

