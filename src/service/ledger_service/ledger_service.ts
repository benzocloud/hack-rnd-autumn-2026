import type { IAccount, ICategoryList, ICreateManualTransactionParams, IGetDefaultAccountParams, IListCategoriesParams, IListTransactionsParams, ISetDatedBalanceParams, ITransaction, ITransactionList, IUpdateTransactionCategoryParams } from "@/interfaces";

import api from "../api";


const getDefaultAccount = async ( request: IGetDefaultAccountParams, signal?: AbortSignal ): Promise<IAccount> => {
    const response = await api.request<IAccount>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/account`,
        signal,
    });


    return response.data;
};


const setDatedBalance = async ( request: ISetDatedBalanceParams, signal?: AbortSignal ): Promise<IAccount> => {
    const response = await api.request<IAccount>({
        method: "PUT",
        url: `/companies/${encodeURIComponent(request.company_id)}/account/balance`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const listCategories = async ( request: IListCategoriesParams, signal?: AbortSignal ): Promise<ICategoryList> => {
    const response = await api.request<ICategoryList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/categories`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const listTransactions = async ( request: IListTransactionsParams, signal?: AbortSignal ): Promise<ITransactionList> => {
    const response = await api.request<ITransactionList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/transactions`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
            from: request.from,
            to: request.to,
            direction: request.direction,
            category_id: request.category_id,
        },
    });


    return response.data;
};


const createManualTransaction = async ( request: ICreateManualTransactionParams, signal?: AbortSignal ): Promise<ITransaction> => {
    const response = await api.request<ITransaction>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/transactions`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const updateTransactionCategory = async ( request: IUpdateTransactionCategoryParams, signal?: AbortSignal ): Promise<ITransaction> => {
    const response = await api.request<ITransaction>({
        method: "PATCH",
        url: `/companies/${encodeURIComponent(request.company_id)}/transactions/${encodeURIComponent(request.transaction_id)}/category`,
        signal,
        data: request.data,
    });


    return response.data;
};


const ledgerService = { getDefaultAccount, setDatedBalance, listCategories, listTransactions, createManualTransaction, updateTransactionCategory };


export default ledgerService;
