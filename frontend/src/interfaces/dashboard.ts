import type { IAccount, ICompany, IForecastSucceeded } from "@/interfaces";


type ForecastPeriod = 30 | 60 | 90;


interface IDashboardData {
    company: ICompany | null;
    account: IAccount | null;
    forecast: IForecastSucceeded | null;
}


export type { ForecastPeriod, IDashboardData };
