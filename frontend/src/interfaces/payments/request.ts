import type { IPositiveMoney } from "@/interfaces";


interface ICreatePaymentRequest {
    direction: "inflow" | "outflow";
    amount: IPositiveMoney;
    scheduled_date: string;
    counterparty_name: string;
    description: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
}


type IUpdatePaymentRequest = ({
    direction: "inflow" | "outflow";
    amount?: IPositiveMoney;
    scheduled_date?: string;
    counterparty_name?: string;
    description?: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
    version: string;
} | {
    direction?: "inflow" | "outflow";
    amount: IPositiveMoney;
    scheduled_date?: string;
    counterparty_name?: string;
    description?: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
    version: string;
} | {
    direction?: "inflow" | "outflow";
    amount?: IPositiveMoney;
    scheduled_date: string;
    counterparty_name?: string;
    description?: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
    version: string;
} | {
    direction?: "inflow" | "outflow";
    amount?: IPositiveMoney;
    scheduled_date?: string;
    counterparty_name: string;
    description?: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
    version: string;
} | {
    direction?: "inflow" | "outflow";
    amount?: IPositiveMoney;
    scheduled_date?: string;
    counterparty_name?: string;
    description: string;
    category_id?: string | null;
    reconciliation_group_id?: string | null;
    version: string;
} | {
    direction?: "inflow" | "outflow";
    amount?: IPositiveMoney;
    scheduled_date?: string;
    counterparty_name?: string;
    description?: string;
    category_id: string | null;
    version: string;
});


interface ICreateMatchRequest {
    transaction_id: string;
    amount: IPositiveMoney;
    payment_version: string;
    transaction_version: string;
}


interface IListPaymentsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
    from?: string;
    to?: string;
    status?: "planned" | "partially_matched" | "matched" | "cancelled";
}


interface ICreatePaymentParams {
    company_id: string;
    idempotency_key: string;
    data: ICreatePaymentRequest;
}


interface IGetPaymentParams {
    company_id: string;
    payment_id: string;
}


interface IUpdatePaymentParams {
    company_id: string;
    payment_id: string;
    data: IUpdatePaymentRequest;
}


interface ICancelPaymentParams {
    company_id: string;
    payment_id: string;
    version: string;
}


interface IListPaymentMatchesParams {
    company_id: string;
    payment_id: string;
    limit?: number;
    cursor?: string;
}


interface IMatchPaymentParams {
    company_id: string;
    payment_id: string;
    idempotency_key: string;
    data: ICreateMatchRequest;
}


interface IRemovePaymentMatchParams {
    company_id: string;
    payment_id: string;
    match_id: string;
    payment_version: string;
}


export type { ICreatePaymentRequest, IUpdatePaymentRequest, ICreateMatchRequest, IListPaymentsParams, ICreatePaymentParams, IGetPaymentParams, IUpdatePaymentParams, ICancelPaymentParams, IListPaymentMatchesParams, IMatchPaymentParams, IRemovePaymentMatchParams };


// OpenAPI CreateReconciliationGroupRequest: exactly one explicit confirmation branch.
type ICreateReconciliationGroupRequest = {
    cutoff_date: string;
    horizon_start: string;
    horizon_end: string;
} & ({ kind: "additional"; additional: { confirmed: true } } | {
    kind: "replacement";
    replacement: { complete_for_horizon: true; historical_operations: Array<{ transaction_id: string; version: string }> };
});

export type { ICreateReconciliationGroupRequest };
