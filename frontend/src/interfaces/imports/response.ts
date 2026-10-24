import type { IErrorDetail, IImportIssue, IImportRecordKinds, IMoney, ITransactionSourceBinding } from "@/interfaces";


interface IBalanceObservation {
    date: string;
    balance: IMoney;
    source: "statement_eod" | "manual";
}


interface IImportPreview {
    source_company_id: string;
    source_system: string;
    account_id: string;
    currency: "RUB";
    period_start: string;
    period_end: string;
    row_count: number;
    excluded_other_company_row_count: number;
    new_transaction_count: number;
    already_present_count: number;
    new_payment_count: number;
    blocking_issue_count: number;
    warning_count: number;
    issues: Array<IImportIssue>;
    issues_truncated: boolean;
    balance_observations: Array<IBalanceObservation>;
    base_data_version: string;
    can_commit: boolean;
    preview_version: string;
    record_kinds: IImportRecordKinds;
    transaction_source: (ITransactionSourceBinding | null);
    would_bind_transaction_source: (ITransactionSourceBinding | null);
}


interface IImportBase {
    id: string;
    account_id: string;
    source_company_id: string;
    version: string;
    created_at: string;
    updated_at: string;
}


type IImport = IImportBase & ({
    status: "queued";
    preview: null;
    applied_data_version: null;
    error: null;
} | {
    status: "processing";
    preview: null;
    applied_data_version: null;
    error: null;
} | {
    status: "preview_ready";
    preview: IImportPreview;
    applied_data_version: null;
    error: null;
} | {
    status: "applied";
    preview: IImportPreview;
    applied_data_version: string;
    error: null;
} | {
    status: "failed";
    preview: null;
    applied_data_version: null;
    error: IErrorDetail;
}) & ({
    source_kind: "file";
    adapter: "dataset_v1";
    connection_id: null;
    record_kinds: [ "transactions", "balance_observations" ];
} | {
    source_kind: "mock_external";
    adapter: string;
    connection_id: string | null;
    record_kinds: IImportRecordKinds;
});


interface IImportList {
    items: Array<IImport>;
    next_cursor: string | null;
}


interface IImportIssueList {
    items: Array<IImportIssue>;
    next_cursor: string | null;
}


export type { IBalanceObservation, IImportPreview, IImport, IImportList, IImportIssueList };
