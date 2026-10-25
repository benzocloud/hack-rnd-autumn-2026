const descriptions: Record<string, string> = {
    csv_history_completeness_unverified: "Прогноз рассчитан по загруженным операциям. Полнота банковской истории не подтверждена.",
    unobserved_days_unknown: "Дни без загруженных операций остаются неизвестными; это не подтверждённое отсутствие движений.",
    observed_operations_mean_per_calendar_day: "Оценка использует средние поступления и расходы из загруженных операций за 90 календарных дней.",
    historical_calendar_unavailable: "Будущие обязательства, не представленные в истории, могут изменить результат.",
    historical_availability_unverified: "Не подтверждено, когда сведения об операциях стали доступны.",
    synthetic_calendar_scenario_not_historical_fact: "Календарь синтетический: это отдельный сценарий, а не подтверждённая историческая доступность обязательств.",
    dataset_coverage_assumed_not_bank_certified: "Полнота принята как допущение демонстрационного набора; банк её не подтверждал.",
    not_calibrated: "Вероятность кассового разрыва не откалибрована и не оценивается.",
    "Forecast uses available imported inflows/outflows divided by 90 calendar days.": "Расчёт делит суммы загруженных поступлений и расходов на 90 календарных дней.",
    "Missing operations can understate either flow; unknown dates are not verified zero-activity days.": "Пропущенные операции могут занизить поступления или расходы. Неизвестные дни не считаются подтверждёнными днями без движений.",
    "The projection excludes unknown future obligations and has no calibrated probability or interval.": "Неизвестные будущие обязательства не учтены. Достоверная вероятность и интервал прогноза не оценены.",
};

export const forecastQualityDescription = ( value: string ): string => descriptions[value] || value;
