interface ICreateCompanyRequest {
    name: string;
    inn?: string | null;
    currency: "RUB";
    timezone: "Europe/Moscow";
}


type IUpdateCompanyRequest = ({
    name: string;
    inn?: string | null;
    version: string;
} | {
    name?: string;
    inn: string | null;
    version: string;
});


interface IListCompaniesParams {
    limit?: number;
    cursor?: string;
}


interface ICreateCompanyParams {
    idempotency_key: string;
    data: ICreateCompanyRequest;
}


interface IGetCompanyParams {
    company_id: string;
}


interface IUpdateCompanyParams {
    company_id: string;
    data: IUpdateCompanyRequest;
}


export type { ICreateCompanyRequest, IUpdateCompanyRequest, IListCompaniesParams, ICreateCompanyParams, IGetCompanyParams, IUpdateCompanyParams };
