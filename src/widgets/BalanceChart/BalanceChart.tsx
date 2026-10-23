"use client";

import useMediaQuery from "@mui/material/useMediaQuery";
import { CartesianGrid, Line, LineChart, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import formatMoney from "@/helpers/formatMoney";
import type { IForecastPoint } from "@/interfaces";

import styles from "./style.module.scss";


interface IProps {
    points: IForecastPoint[];
}


const dayFormatter = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", timeZone: "UTC" });
const numberFormatter = new Intl.NumberFormat("ru-RU", { notation: "compact", maximumFractionDigits: 1 });


const BalanceChart = ( { points }: IProps ) => {
    const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
    const data = points.map(( point ) => ({ date: point.date, value: Number(point.balance.amount_minor) / 100, amountMinor: point.balance.amount_minor }));


    return (
        <div className={styles.chart} role="group" aria-label={`Прогноз остатка на счёте в рублях за ${points.length} дней. По горизонтали — дни, по вертикали — сумма.`}>
            <ResponsiveContainer width="100%" height="100%" minWidth={0}>
                <LineChart data={data} margin={{ top: 20, right: 36, bottom: 16, left: 0 }} accessibilityLayer>
                    <CartesianGrid stroke="var(--chart-grid)" strokeOpacity={0.2} vertical={false} />
                    <XAxis dataKey="date" tickFormatter={( value: string ) => dayFormatter.format(new Date(`${value}T00:00:00Z`))} tick={{ fill: "var(--chart-text)", fontSize: 11 }} tickLine={false} axisLine={false} minTickGap={24} tickMargin={10} interval="preserveStartEnd" />
                    <YAxis tickFormatter={( value: number ) => numberFormatter.format(value)} tick={{ fill: "var(--chart-text)", fontSize: 11 }} tickLine={false} axisLine={false} width={62} tickMargin={8} domain={[ "auto", "auto" ]} />
                    <ReferenceLine y={0} stroke="var(--chart-grid)" strokeOpacity={0.7} strokeDasharray="4 4" ifOverflow="extendDomain" />
                    <Tooltip
                        isAnimationActive={false}
                        cursor={{ stroke: "var(--chart-grid)", strokeOpacity: 0.5 }}
                        content={( { active, label } ) => {
                            const point = data.find(( item ) => item.date === label);

                            return active && point ? (
                                <div className={styles.tooltip}>
                                    <span>{ dayFormatter.format(new Date(`${point.date}T00:00:00Z`)) }</span>
                                    <strong>{ formatMoney(point.amountMinor) }</strong>
                                </div>
                            ) : null;
                        }}
                    />
                    <Line type="monotone" dataKey="value" name="Остаток на счёте" stroke="var(--chart-line)" strokeWidth={3} strokeLinecap="round" dot={false} activeDot={{ r: 5, fill: "var(--chart-line)", stroke: "var(--chart-text)", strokeWidth: 2 }} isAnimationActive={!reducedMotion} animationDuration={900} animationEasing="ease-in" />
                </LineChart>
            </ResponsiveContainer>
        </div>
    );
};


export default BalanceChart;
