interface ICreateForecastRequest {
    cutoff_date: string;
    horizon_days: number;
}


interface IListForecastsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
    include_scenarios?: boolean;
}


interface ICreateForecastParams {
    company_id: string;
    idempotency_key: string;
    data: ICreateForecastRequest;
}


interface IGetForecastParams {
    company_id: string;
    forecast_id: string;
}


interface ICompareForecastsParams {
    company_id: string;
    forecast_id: string;
    reference_forecast_id: string;
}


interface IGetPlanFactParams {
    company_id: string;
    forecast_id: string;
    from?: string;
    to?: string;
}


export type { ICreateForecastRequest, IListForecastsParams, ICreateForecastParams, IGetForecastParams, ICompareForecastsParams, IGetPlanFactParams };
