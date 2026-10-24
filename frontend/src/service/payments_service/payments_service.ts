import type { ICancelPaymentParams, ICreatePaymentParams, IGetPaymentParams, IListPaymentMatchesParams, IListPaymentsParams, IMatch, IMatchList, IMatchPaymentParams, IPayment, IPaymentList, IRemovePaymentMatchParams, IUpdatePaymentParams } from "@/interfaces";

import api from "../api";


const listPayments = async ( request: IListPaymentsParams, signal?: AbortSignal ): Promise<IPaymentList> => {
    const response = await api.request<IPaymentList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
            from: request.from,
            to: request.to,
            status: request.status,
        },
    });


    return response.data;
};


const createPayment = async ( request: ICreatePaymentParams, signal?: AbortSignal ): Promise<IPayment> => {
    const response = await api.request<IPayment>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const getPayment = async ( request: IGetPaymentParams, signal?: AbortSignal ): Promise<IPayment> => {
    const response = await api.request<IPayment>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}`,
        signal,
    });


    return response.data;
};


const updatePayment = async ( request: IUpdatePaymentParams, signal?: AbortSignal ): Promise<IPayment> => {
    const response = await api.request<IPayment>({
        method: "PATCH",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}`,
        signal,
        data: request.data,
    });


    return response.data;
};


const cancelPayment = async ( request: ICancelPaymentParams, signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "DELETE",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}`,
        signal,
        params: {
            version: request.version,
        },
    });


    return response.data;
};


const listPaymentMatches = async ( request: IListPaymentMatchesParams, signal?: AbortSignal ): Promise<IMatchList> => {
    const response = await api.request<IMatchList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}/matches`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const matchPayment = async ( request: IMatchPaymentParams, signal?: AbortSignal ): Promise<IMatch> => {
    const response = await api.request<IMatch>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}/matches`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const removePaymentMatch = async ( request: IRemovePaymentMatchParams, signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "DELETE",
        url: `/companies/${encodeURIComponent(request.company_id)}/payments/${encodeURIComponent(request.payment_id)}/matches/${encodeURIComponent(request.match_id)}`,
        signal,
        params: {
            payment_version: request.payment_version,
        },
    });


    return response.data;
};


const paymentsService = { listPayments, createPayment, getPayment, updatePayment, cancelPayment, listPaymentMatches, matchPayment, removePaymentMatch };


export default paymentsService;
