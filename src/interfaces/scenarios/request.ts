import type { IResponseAction, IScenarioChange } from "@/interfaces";


type ICreateScenarioRequest = (IIndependentScenarioRequest | IResponseScenarioRequest);


interface IIndependentScenarioRequest {
    name: string;
    base_forecast_id: string;
    changes: Array<IScenarioChange>;
}


interface IResponseScenarioRequest {
    name: string;
    comparison_parent_scenario_id: string;
    response_action: IResponseAction;
}


interface IListScenariosParams {
    company_id: string;
    limit?: number;
    cursor?: string;
    base_forecast_id?: string;
}


interface ICreateScenarioParams {
    company_id: string;
    idempotency_key: string;
    data: ICreateScenarioRequest;
}


interface IGetScenarioParams {
    company_id: string;
    scenario_id: string;
}


export type { ICreateScenarioRequest, IIndependentScenarioRequest, IResponseScenarioRequest, IListScenariosParams, ICreateScenarioParams, IGetScenarioParams };
