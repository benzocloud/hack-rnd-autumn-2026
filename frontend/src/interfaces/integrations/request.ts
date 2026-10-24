import type { IImportRecordKinds } from "@/interfaces";


interface ICreateConnectionRequest {
    provider_id: string;
    display_name: string;
    source_company_id: string;
}


interface IStartSyncRequest {
    period_start: string;
    period_end: string;
    record_kinds: IImportRecordKinds;
}


interface IListExternalProvidersParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface IListConnectionsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface ICreateConnectionParams {
    company_id: string;
    idempotency_key: string;
    data: ICreateConnectionRequest;
}


interface IDisableConnectionParams {
    company_id: string;
    connection_id: string;
}


interface ISyncConnectionParams {
    company_id: string;
    connection_id: string;
    idempotency_key: string;
    data: IStartSyncRequest;
}


export type { ICreateConnectionRequest, IStartSyncRequest, IListExternalProvidersParams, IListConnectionsParams, ICreateConnectionParams, IDisableConnectionParams, ISyncConnectionParams };
