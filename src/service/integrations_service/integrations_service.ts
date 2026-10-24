import type { IConnection, IConnectionList, ICreateConnectionParams, IDisableConnectionParams, IExternalProviderList, IImportAccepted, IListConnectionsParams, IListExternalProvidersParams, ISyncConnectionParams } from "@/interfaces";

import api from "../api";


const listExternalProviders = async ( request: IListExternalProvidersParams, signal?: AbortSignal ): Promise<IExternalProviderList> => {
    const response = await api.request<IExternalProviderList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/external-providers`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const listConnections = async ( request: IListConnectionsParams, signal?: AbortSignal ): Promise<IConnectionList> => {
    const response = await api.request<IConnectionList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/connections`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const createConnection = async ( request: ICreateConnectionParams, signal?: AbortSignal ): Promise<IConnection> => {
    const response = await api.request<IConnection>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/connections`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const disableConnection = async ( request: IDisableConnectionParams, signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "DELETE",
        url: `/companies/${encodeURIComponent(request.company_id)}/connections/${encodeURIComponent(request.connection_id)}`,
        signal,
    });


    return response.data;
};


const syncConnection = async ( request: ISyncConnectionParams, signal?: AbortSignal ): Promise<IImportAccepted> => {
    const response = await api.request<IImportAccepted>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/connections/${encodeURIComponent(request.connection_id)}/sync`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const integrationsService = { listExternalProviders, listConnections, createConnection, disableConnection, syncConnection };


export default integrationsService;
