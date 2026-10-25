import type { ICommitImportParams, IGetImportParams, IImport, IImportAccepted, IImportIssueList, IImportList, IListImportIssuesParams, IListImportsParams, IUploadImportParams } from "@/interfaces";

import api from "../api";


const listImports = async ( request: IListImportsParams, signal?: AbortSignal ): Promise<IImportList> => {
    const response = await api.request<IImportList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/imports`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const uploadImport = async ( request: IUploadImportParams, signal?: AbortSignal, onProgress?: ( percent: number ) => void ): Promise<IImportAccepted> => {
    const data = new FormData();

    data.append("file", request.data.file);
    data.append("adapter", request.data.adapter);
    data.append("source_company_id", request.data.source_company_id);
    data.append("account_id", request.data.account_id);
    data.append("currency", request.data.currency);


    const response = await api.request<IImportAccepted>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/imports`,
        signal,
        data: data,
        onUploadProgress: ( event ) => { if (event.total) { onProgress?.(Math.min(100, Math.round(event.loaded / event.total * 100))); } },
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const getImport = async ( request: IGetImportParams, signal?: AbortSignal ): Promise<IImport> => {
    const response = await api.request<IImport>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/imports/${encodeURIComponent(request.import_id)}`,
        signal,
    });


    return response.data;
};


const listImportIssues = async ( request: IListImportIssuesParams, signal?: AbortSignal ): Promise<IImportIssueList> => {
    const response = await api.request<IImportIssueList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/imports/${encodeURIComponent(request.import_id)}/issues`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const commitImport = async ( request: ICommitImportParams, signal?: AbortSignal ): Promise<IImport> => {
    const response = await api.request<IImport>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/imports/${encodeURIComponent(request.import_id)}/commit`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const importsService = { listImports, uploadImport, getImport, listImportIssues, commitImport };


export default importsService;
