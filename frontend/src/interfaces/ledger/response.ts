import type { IMoney, IPositiveMoney, ISource, ITransactionSourceBinding } from "@/interfaces";


interface IAccount {
    id: string;
    company_id: string;
    name: string;
    currency: "RUB";
    balance: (IMoney | null);
    balance_as_of: string | null;
    balance_source: "statement_eod" | "manual" | null;
    data_version: string;
    transaction_source: (ITransactionSourceBinding | null);
}


interface ICategory {
    id: string;
    name: string;
    direction: "inflow" | "outflow" | null;
    semantic_kind: "operating_expense" | "sales_receipt" | "tax" | "financing" | "own_transfer" | "other";
}


interface ITransaction {
    id: string;
    account_id: string;
    operation_date: string;
    direction: "inflow" | "outflow";
    amount: IPositiveMoney;
    counterparty_inn: string | null;
    counterparty_name: string;
    payment_purpose: string;
    category_id: string | null;
    source: ISource;
    version: string;
    created_at: string;
}


interface ICategoryList {
    items: Array<ICategory>;
    next_cursor: string | null;
}


interface ITransactionList {
    items: Array<ITransaction>;
    next_cursor: string | null;
}


export type { IAccount, ICategory, ITransaction, ICategoryList, ITransactionList };
