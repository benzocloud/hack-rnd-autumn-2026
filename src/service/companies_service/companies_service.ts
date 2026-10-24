import type { ICompany, ICompanyList, ICreateCompanyParams, IGetCompanyParams, IListCompaniesParams, IUpdateCompanyParams } from "@/interfaces";

import api from "../api";


const listCompanies = async ( request: IListCompaniesParams = {}, signal?: AbortSignal ): Promise<ICompanyList> => {
    const response = await api.request<ICompanyList>({
        method: "GET",
        url: "/companies",
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const createCompany = async ( request: ICreateCompanyParams, signal?: AbortSignal ): Promise<ICompany> => {
    const response = await api.request<ICompany>({
        method: "POST",
        url: "/companies",
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const getCompany = async ( request: IGetCompanyParams, signal?: AbortSignal ): Promise<ICompany> => {
    const response = await api.request<ICompany>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}`,
        signal,
    });


    return response.data;
};


const updateCompany = async ( request: IUpdateCompanyParams, signal?: AbortSignal ): Promise<ICompany> => {
    const response = await api.request<ICompany>({
        method: "PATCH",
        url: `/companies/${encodeURIComponent(request.company_id)}`,
        signal,
        data: request.data,
    });


    return response.data;
};


const companiesService = { listCompanies, createCompany, getCompany, updateCompany };


export default companiesService;
