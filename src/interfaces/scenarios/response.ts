import type { IPositiveMoney } from "@/interfaces";


interface IOccurrenceTarget {
    component_id: string;
    occurrence_id: string;
}


interface IDelayReceiptChange {
    id: string;
    type: "delay_receipt";
    target: IOccurrenceTarget;
    amount: IPositiveMoney;
    new_date: string;
}


interface IReduceSalesChange {
    id: string;
    type: "reduce_sales";
    targets: Array<IOccurrenceTarget>;
    period_start: string;
    period_end: string;
    reduction_basis_points: number;
}


interface IOneOffExpenseChange {
    id: string;
    type: "one_off_expense";
    date: string;
    amount: IPositiveMoney;
    description: string;
    category_id?: string | null;
}


interface IExpenseIncreasePercent {
    mode: "percent";
    increase_basis_points: number;
}


interface IExpenseIncreaseAmount {
    mode: "amount";
    new_amount: IPositiveMoney;
}


interface IIncreaseRecurringExpensesChange {
    id: string;
    type: "increase_recurring_expenses";
    targets: Array<IOccurrenceTarget>;
    effective_date: string;
    adjustment: (IExpenseIncreasePercent | IExpenseIncreaseAmount);
}


type IScenarioChange = (IDelayReceiptChange | IReduceSalesChange | IOneOffExpenseChange | IIncreaseRecurringExpensesChange);


interface IResponseAction {
    type: "reschedule_payment_part";
    target: IEffectiveOccurrenceTarget;
    amount: IPositiveMoney;
    new_date: string;
    feasibility_condition: string;
}


interface IScenario {
    id: string;
    name: string;
    base_forecast_id: string;
    comparison_parent_scenario_id: string | null;
    forecast_id: string;
    changes: Array<IScenarioChange>;
    response_action: (IResponseAction | null);
    out_of_horizon_occurrence_ids: Array<string>;
    created_at: string;
}


interface IScenarioAccepted {
    id: string;
    forecast_id: string;
    status: "queued";
    status_url: string;
    poll_after_ms: number;
}


interface IScenarioList {
    items: Array<IScenario>;
    next_cursor: string | null;
}


interface IEffectiveOccurrenceTarget {
    effective_occurrence_id: string;
}


export type { IOccurrenceTarget, IDelayReceiptChange, IReduceSalesChange, IOneOffExpenseChange, IExpenseIncreasePercent, IExpenseIncreaseAmount, IIncreaseRecurringExpensesChange, IScenarioChange, IResponseAction, IScenario, IScenarioAccepted, IScenarioList, IEffectiveOccurrenceTarget };
