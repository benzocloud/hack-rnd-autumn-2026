import type { ICreateScenarioParams, IGetScenarioParams, IListScenariosParams, IScenario, IScenarioAccepted, IScenarioList } from "@/interfaces";

import api from "../api";


const listScenarios = async ( request: IListScenariosParams, signal?: AbortSignal ): Promise<IScenarioList> => {
    const response = await api.request<IScenarioList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/scenarios`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
            base_forecast_id: request.base_forecast_id,
        },
    });


    return response.data;
};


const createScenario = async ( request: ICreateScenarioParams, signal?: AbortSignal ): Promise<IScenarioAccepted> => {
    const response = await api.request<IScenarioAccepted>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/scenarios`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const getScenario = async ( request: IGetScenarioParams, signal?: AbortSignal ): Promise<IScenario> => {
    const response = await api.request<IScenario>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/scenarios/${encodeURIComponent(request.scenario_id)}`,
        signal,
    });


    return response.data;
};


const scenariosService = { listScenarios, createScenario, getScenario };


export default scenariosService;
