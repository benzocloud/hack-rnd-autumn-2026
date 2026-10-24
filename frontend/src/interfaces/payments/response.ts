import type { INonnegativeMoney, IPositiveMoney, ISource } from "@/interfaces";


interface IPayment {
    id: string;
    account_id: string;
    direction: "inflow" | "outflow";
    amount: IPositiveMoney;
    remaining_amount: INonnegativeMoney;
    scheduled_date: string;
    counterparty_name: string;
    description: string;
    category_id: string | null;
    source: ISource;
    status: "planned" | "partially_matched" | "matched" | "cancelled";
    version: string;
    created_at: string;
}


interface IMatch {
    id: string;
    payment_id: string;
    transaction_id: string;
    amount: IPositiveMoney;
    created_at: string;
}


interface IPaymentList {
    items: Array<IPayment>;
    next_cursor: string | null;
}


interface IMatchList {
    items: Array<IMatch>;
    next_cursor: string | null;
}


export type { IPayment, IMatch, IPaymentList, IMatchList };
