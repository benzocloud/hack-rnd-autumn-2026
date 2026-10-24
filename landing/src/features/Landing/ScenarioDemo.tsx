"use client";

import { useState } from "react";

import { makeDemoForecast } from "@/helpers/demoForecast";
import formatMoney from "@/helpers/formatMoney";
import type { DemoScenario } from "@/interfaces";
import BalanceChart from "@/widgets/BalanceChart/BalanceChart";
import Icon from "@/widgets/Icon/Icon";

import styles from "./style.module.scss";


const scenarios: { id: DemoScenario; title: string; detail: string; icon: string; description: string }[] = [
    { id: "delay", title: "Клиент заплатит позже", detail: "Поступление 180 000 ₽ задержится на 11 дней", icon: "calendar", description: "В примере переносим поступление на более позднюю дату. Деньги придут, но до этого остаток может оказаться ниже нуля." },
    { id: "sales", title: "Выручка снизится", detail: "Ежедневные поступления станут на 6 500 ₽ меньше", icon: "chart-line", description: "В примере уменьшаем поступления после пятого дня. Небольшое ежедневное изменение постепенно увеличивает дефицит." },
    { id: "expense", title: "Появится крупный расход", detail: "Разовая покупка оборудования на 250 000 ₽", icon: "arrows-exchange", description: "В примере добавляем разовый расход на десятый день. Сравните остаток, прежде чем планировать крупную покупку." },
    { id: "recurring", title: "Регулярные расходы вырастут", detail: "Еженедельные расходы увеличатся на 45 000 ₽", icon: "refresh", description: "В примере добавляем расходы каждую неделю. На графике видно, как регулярные затраты влияют на будущий остаток." },
];


const ScenarioDemo = () => {
    const [ selected, setSelected ] = useState<DemoScenario>("delay");
    const base = makeDemoForecast(30);
    const points = makeDemoForecast(30, selected);
    const scenario = scenarios.find(( item ) => item.id === selected)!;
    const minimum = points.reduce(( value, point ) => BigInt(point.balance.amount_minor) < value ? BigInt(point.balance.amount_minor) : value, BigInt(points[0].balance.amount_minor));

    return (
        <div className={styles.scenarioGrid}>
            <div className={styles.scenarioOptions} role="group" aria-label="Выберите пример сценария">{ scenarios.map(( item ) => <button type="button" className={styles.scenarioOption} key={item.id} aria-pressed={selected === item.id} onClick={() => setSelected(item.id)}><span className={styles.scenarioOptionIcon}><Icon name={item.icon} /></span><span><strong>{ item.title }</strong><span>{ item.detail }</span></span><span className={styles.radioMark} /></button>) }</div>
            <div className={styles.scenarioChart}>
                <div className={styles.scenarioChartHeading}><div><span className={styles.eyebrow}>СРАВНЕНИЕ ВАРИАНТОВ</span><h3>Одно решение. Два будущих.</h3></div><span className={styles.demoBadge}>Демо</span></div>
                <BalanceChart points={points} comparison={base} />
                <div className={styles.scenarioLegend}><span><i />Сценарий</span><span><i />Исходный прогноз</span></div>
                <div className={styles.scenarioResult} aria-live="polite"><span>Минимальный остаток<strong>{ formatMoney(minimum.toString()) }</strong></span><p>{ scenario.description }</p></div>
                <p className={styles.demoDisclaimer}>Локальный пример на демонстрационных данных. Backend и ML не вызываются.</p>
            </div>
        </div>
    );
};


export default ScenarioDemo;

