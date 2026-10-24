import type { IMoney, IPositiveMoney } from "@/interfaces";


interface ISetBalanceRequest {
    balance: IMoney;
    as_of: string;
    expected_data_version: string;
    note: string;
}


interface ICreateTransactionRequest {
    operation_date: string;
    direction: "inflow" | "outflow";
    amount: IPositiveMoney;
    counterparty_inn?: string | null;
    counterparty_name: string;
    payment_purpose: string;
    category_id?: string | null;
    expected_data_version: string;
}


interface IUpdateCategoryRequest {
    category_id: string | null;
    version: string;
}


interface IGetDefaultAccountParams {
    company_id: string;
}


interface ISetDatedBalanceParams {
    company_id: string;
    idempotency_key: string;
    data: ISetBalanceRequest;
}


interface IListCategoriesParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface IListTransactionsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
    from?: string;
    to?: string;
    direction?: "inflow" | "outflow";
    category_id?: string;
}


interface ICreateManualTransactionParams {
    company_id: string;
    idempotency_key: string;
    data: ICreateTransactionRequest;
}


interface IUpdateTransactionCategoryParams {
    company_id: string;
    transaction_id: string;
    data: IUpdateCategoryRequest;
}


export type { ISetBalanceRequest, ICreateTransactionRequest, IUpdateCategoryRequest, IGetDefaultAccountParams, ISetDatedBalanceParams, IListCategoriesParams, IListTransactionsParams, ICreateManualTransactionParams, IUpdateTransactionCategoryParams };
