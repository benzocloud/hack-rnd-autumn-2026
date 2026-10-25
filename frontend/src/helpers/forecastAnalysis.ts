import type { ICategorySource, IForecastAnalysis, IRecurringHypothesis } from "@/interfaces";


const categorySourceLabels: Record<ICategorySource, string> = {
    manual: "Вручную",
    rules: "Анализ",
    unknown: "Не определено",
};


const weekendShiftLabels: Record<IRecurringHypothesis["weekend_shift"], string> = {
    none: "без сдвига",
    previous: "на предыдущий рабочий день",
    next: "на следующий рабочий день",
};


const categorySourceLabel = ( source: ICategorySource ): string => categorySourceLabels[source];


const categorySourceTone = ( source: ICategorySource ): "success" | "neutral" | "warning" => {
    if (source === "manual") {
        return "success";
    }

    if (source === "rules") {
        return "neutral";
    }

    return "warning";
};


const directionLabel = ( direction: "inflow" | "outflow" ): string => direction === "inflow" ? "Поступление" : "Списание";


const weekendShiftLabel = ( shift: IRecurringHypothesis["weekend_shift"] ): string => weekendShiftLabels[shift];


const hypothesisScheduleLabel = ( hypothesis: IRecurringHypothesis ): string => {
    if (hypothesis.month_end === true) {
        return `конец месяца · ${weekendShiftLabel(hypothesis.weekend_shift)}`;
    }

    return `${hypothesis.day_of_month}-е число · ${weekendShiftLabel(hypothesis.weekend_shift)}`;
};


const hasForecastAnalysis = ( analysis: IForecastAnalysis | null | undefined ): analysis is IForecastAnalysis => analysis != null;


export { categorySourceLabel, categorySourceTone, directionLabel, weekendShiftLabel, hypothesisScheduleLabel, hasForecastAnalysis };
