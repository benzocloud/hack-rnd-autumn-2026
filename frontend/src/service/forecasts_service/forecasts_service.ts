import type { ICompareForecastsParams, IComparison, ICreateForecastParams, IForecast, IForecastAccepted, IForecastList, IGetForecastParams, IGetPlanFactParams, IListForecastsParams, IPlanFact } from "@/interfaces";

import api from "../api";


const listForecasts = async ( request: IListForecastsParams, signal?: AbortSignal ): Promise<IForecastList> => {
    const response = await api.request<IForecastList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/forecasts`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
            include_scenarios: request.include_scenarios,
        },
    });


    return response.data;
};


const createForecast = async ( request: ICreateForecastParams, signal?: AbortSignal ): Promise<IForecastAccepted> => {
    const response = await api.request<IForecastAccepted>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/forecasts`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const getForecast = async ( request: IGetForecastParams, signal?: AbortSignal ): Promise<IForecast> => {
    const response = await api.request<IForecast>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/forecasts/${encodeURIComponent(request.forecast_id)}`,
        signal,
    });


    return response.data;
};


const compareForecasts = async ( request: ICompareForecastsParams, signal?: AbortSignal ): Promise<IComparison> => {
    const response = await api.request<IComparison>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/forecasts/${encodeURIComponent(request.forecast_id)}/comparison`,
        signal,
        params: {
            reference_forecast_id: request.reference_forecast_id,
        },
    });


    return response.data;
};


const getPlanFact = async ( request: IGetPlanFactParams, signal?: AbortSignal ): Promise<IPlanFact> => {
    const response = await api.request<IPlanFact>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/forecasts/${encodeURIComponent(request.forecast_id)}/plan-fact`,
        signal,
        params: {
            from: request.from,
            to: request.to,
        },
    });


    return response.data;
};


const forecastsService = { listForecasts, createForecast, getForecast, compareForecasts, getPlanFact };


export default forecastsService;
