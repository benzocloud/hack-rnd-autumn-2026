import type { IErrorDetail, IImportIssue, IMoney, INonnegativeMoney, IPositiveMoney, ISource } from "@/interfaces";


interface IForecastReadiness {
    status: "ready" | "limited" | "insufficient";
    history_days: number;
    observed_days: number;
    quality_issues: Array<IImportIssue>;
    limitations: Array<string>;
}


interface IModelContext {
    id: string;
    model_name: string;
    model_version: string;
    preprocessing_version: string;
    configuration_id: string;
    random_seed: string | null;
    trajectory_count: number | null;
}


interface IForecastInput {
    snapshot_id: string;
    data_version: string;
    cutoff_date: string;
    data_as_of: string;
    start_date: string;
    end_date: string;
    horizon_days: number;
    currency: "RUB";
    account_id: string;
    opening_balance: IMoney;
    balance_as_of: string;
    risk_definition: "below_zero_eod";
    model_context: IModelContext;
    history_assessment?: {
        rule_version: "csv_history_quality_v1";
        from_date: string;
        through_date: string;
        data_version: string;
        operation_days: number;
        balance_days: number;
        limitations: Array<string>;
    } | null;
}


interface IForecastAccepted {
    id: string;
    status: "queued";
    status_url: string;
    poll_after_ms: number;
}


type IProbability = ({
    value: null;
    unavailable_reason: string;
} | {
    value: number;
    unavailable_reason: null;
});


interface ICashGap {
    date: string;
    shortfall: IPositiveMoney;
}


interface IForecastMetrics {
    first_gap: (ICashGap | null);
    max_shortfall: INonnegativeMoney;
    minimum_balance: IMoney;
    closing_balance: IMoney;
    gap_probability: IProbability;
}


interface IForecastPoint {
    date: string;
    inflow: INonnegativeMoney;
    outflow: INonnegativeMoney;
    balance: IMoney;
    balance_lower: (IMoney | null);
    balance_upper: (IMoney | null);
    negative_balance_probability: IProbability;
}


interface IEvidence {
    type: "payment" | "transaction" | "forecast_component" | "quality_issue";
    resource_id: string;
    source: (ISource | null);
    description: string;
}


interface IRecommendation {
    id: string;
    title: string;
    explanation: string;
    feasibility_condition: string;
    evidence: Array<IEvidence>;
    suggested_action: "reschedule_payment" | "request_early_receipt" | "reduce_discretionary_expense" | null;
}


interface IForecastComponent {
    id: string;
    occurrence_id: string;
    kind: "known_payment" | "ml_sales_receipt" | "recurring_expense" | "scenario_expense";
    direction: "inflow" | "outflow";
    date: string;
    amount: IPositiveMoney;
    remaining_amount: INonnegativeMoney;
    category_id: string | null;
    payment_id: string | null;
    recurrence_group_id: string | null;
    source: (ISource | null);
    scenario_eligible: boolean;
}


interface IScenarioCapabilities {
    delay_receipt: boolean;
    reduce_sales: boolean;
    one_off_expense: boolean;
    increase_recurring_expenses: boolean;
    unavailable_reason: string | null;
}


interface IForecastResult {
    metrics: IForecastMetrics;
    daily_points: Array<IForecastPoint>;
    components: Array<IForecastComponent>;
    scenario_capabilities: IScenarioCapabilities;
    recommendations: Array<IRecommendation>;
    assumptions: Array<string>;
    effective_occurrences: Array<IEffectiveOccurrence>;
}


interface IForecastPending {
    id: string;
    company_id: string;
    created_at: string;
    completed_at: null;
    is_stale: boolean;
    current_data_version: string;
    input: IForecastInput;
    readiness: IForecastReadiness;
    scenario_id: string | null;
    status: "queued" | "running";
    result: null;
    error: null;
}


interface IForecastSucceeded {
    id: string;
    company_id: string;
    created_at: string;
    completed_at: string;
    is_stale: boolean;
    current_data_version: string;
    input: IForecastInput;
    readiness: IForecastReadiness;
    scenario_id: string | null;
    status: "succeeded";
    result: IForecastResult;
    error: null;
}


interface IForecastFailed {
    id: string;
    company_id: string;
    created_at: string;
    completed_at: string;
    is_stale: boolean;
    current_data_version: string;
    input: IForecastInput;
    readiness: IForecastReadiness;
    scenario_id: string | null;
    status: "failed";
    result: null;
    error: IErrorDetail;
}


type IForecast = (IForecastPending | IForecastSucceeded | IForecastFailed);


interface IForecastSummary {
    id: string;
    status: "queued" | "running" | "succeeded" | "failed";
    cutoff_date: string;
    start_date: string;
    end_date: string;
    created_at: string;
    is_stale: boolean;
    scenario_id: string | null;
}


interface IComparison {
    reference_forecast_id: string;
    candidate_forecast_id: string;
    snapshot_id: string;
    model_context_id: string;
    is_stale: boolean;
    reference_metrics: IForecastMetrics;
    candidate_metrics: IForecastMetrics;
    delta_max_shortfall: IMoney;
    delta_minimum_balance: IMoney;
    delta_closing_balance: IMoney;
    delta_gap_probability: number | null;
    delta_first_gap_days: number | null;
}


interface IActualCoverage {
    status: "complete" | "partial" | "unavailable";
    observed_through: string | null;
    missing_dates: Array<string>;
    warnings: Array<string>;
}


interface IPlanFactPoint {
    date: string;
    planned_inflow: INonnegativeMoney;
    planned_outflow: INonnegativeMoney;
    planned_balance: IMoney;
    actual_inflow: (INonnegativeMoney | null);
    actual_outflow: (INonnegativeMoney | null);
    actual_balance: (IMoney | null);
    balance_delta: (IMoney | null);
    coverage: "complete" | "partial" | "unavailable";
}


interface IPlanFact {
    forecast_id: string;
    actual_data_version: string;
    period_start: string;
    period_end: string;
    coverage: IActualCoverage;
    daily_points: Array<IPlanFactPoint>;
    mean_absolute_balance_error: (INonnegativeMoney | null);
}


interface IForecastList {
    items: Array<IForecastSummary>;
    next_cursor: string | null;
}


interface IEffectiveOccurrence {
    effective_occurrence_id: string;
    component_id: string;
    occurrence_id: string;
    payment_id: string | null;
    direction: "inflow" | "outflow";
    date: string;
    remaining_amount: INonnegativeMoney;
    in_horizon: boolean;
    response_action_eligible: boolean;
}


export type { IForecastReadiness, IModelContext, IForecastInput, IForecastAccepted, IProbability, ICashGap, IForecastMetrics, IForecastPoint, IEvidence, IRecommendation, IForecastComponent, IScenarioCapabilities, IForecastResult, IForecastPending, IForecastSucceeded, IForecastFailed, IForecast, IForecastSummary, IComparison, IActualCoverage, IPlanFactPoint, IPlanFact, IForecastList, IEffectiveOccurrence };
