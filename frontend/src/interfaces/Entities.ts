interface IMoney {
    amount_minor: string;
    currency: "RUB";
}


interface IPositiveMoney {
    amount_minor: string;
    currency: "RUB";
}


interface INonnegativeMoney {
    amount_minor: string;
    currency: "RUB";
}


interface IFieldError {
    field: string;
    code: string;
    message: string;
}


interface IErrorDetail {
    code: string;
    message: string;
    field_errors: Array<IFieldError>;
    request_id: string;
}


interface IError {
    error: IErrorDetail;
}


interface IUser {
    id: string;
    email: string;
    display_name: string;
    email_verified: boolean;
}


interface ICompany {
    id: string;
    name: string;
    inn: string | null;
    currency: "RUB";
    timezone: "Europe/Moscow";
    membership: {
        role: "owner" | "financier";
    };
    default_account_id: string;
    version: string;
    created_at: string;
}


interface ISource {
    kind: "file" | "manual" | "mock_external";
    source_system: string;
    source_company_id: string | null;
    external_operation_id: string | null;
    external_payment_id: string | null;
    import_id: string | null;
    connection_id: string | null;
}


interface IImportIssue {
    code: "MISSING_COUNTERPARTY_INN" | "EMPTY_PAYMENT_PURPOSE" | "SUSPECTED_CONTENT_DUPLICATE" | "BALANCE_DISCONTINUITY" | "SHORT_HISTORY" | "DUPLICATE_EXTERNAL_ID" | "CONFLICTING_EXTERNAL_ID" | "INVALID_ROW" | "UNSUPPORTED_CURRENCY" | "MULTIPLE_SOURCE_COMPANIES" | "FACTUAL_SOURCE_CONFLICT" | "SOURCE_BINDING_CONFLICT";
    severity: "warning" | "blocking";
    message: string;
    row_number: number | null;
    field: string | null;
    related_external_operation_ids: Array<string>;
}


interface IImportAccepted {
    id: string;
    status: "queued";
    status_url: string;
    poll_after_ms: number;
}


type ITransactionSourceBinding = ({
    mode: "manual";
    source_system: "manual_v1";
    source_company_id: null;
} | {
    mode: "external";
    source_system: string;
    source_company_id: string;
});


type IImportRecordKinds = Array<"transactions" | "planned_payments" | "balance_observations">;


export type { IMoney, IPositiveMoney, INonnegativeMoney, IFieldError, IErrorDetail, IError, IUser, ICompany, ISource, IImportIssue, IImportAccepted, ITransactionSourceBinding, IImportRecordKinds };
