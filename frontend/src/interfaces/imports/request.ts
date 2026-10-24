interface ICommitImportRequest {
    version: string;
    expected_data_version: string;
    acknowledge_warnings: boolean;
}


interface IListImportsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface IUploadImportParams {
    company_id: string;
    idempotency_key: string;
    data: {
        file: File;
        adapter: "dataset_v1";
        source_company_id: string;
        account_id: string;
        currency: "RUB";
    };
}


interface IGetImportParams {
    company_id: string;
    import_id: string;
}


interface IListImportIssuesParams {
    company_id: string;
    import_id: string;
    limit?: number;
    cursor?: string;
}


interface ICommitImportParams {
    company_id: string;
    import_id: string;
    idempotency_key: string;
    data: ICommitImportRequest;
}


export type { ICommitImportRequest, IListImportsParams, IUploadImportParams, IGetImportParams, IListImportIssuesParams, ICommitImportParams };
