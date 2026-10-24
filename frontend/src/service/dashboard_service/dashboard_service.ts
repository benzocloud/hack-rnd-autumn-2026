import { CanceledError } from "axios";

import type { ForecastPeriod, IDashboardData, IForecast, IForecastSucceeded } from "@/interfaces";

import companiesService from "../companies_service/companies_service";
import forecastsService from "../forecasts_service/forecasts_service";
import ledgerService from "../ledger_service/ledger_service";


const waitForForecast = ( duration: number, signal: AbortSignal ) => new Promise<void>(( resolve, reject ) => {
    const finish = () => {
        signal.removeEventListener("abort", abort);
        resolve();
    };
    const abort = () => {
        clearTimeout(timer);
        signal.removeEventListener("abort", abort);
        reject(new CanceledError("Запрос отменён."));
    };
    const timer = setTimeout(finish, Math.min(5000, Math.max(300, duration)));

    signal.addEventListener("abort", abort, { once: true });

    if (signal.aborted) {
        abort();
    }
});


const completeForecast = async ( companyId: string, forecastId: string, pollAfter: number, signal: AbortSignal ): Promise<IForecastSucceeded> => {
    for (let attempt = 0; attempt < 30; attempt++) {
        const forecast: IForecast = await forecastsService.getForecast({ company_id: companyId, forecast_id: forecastId }, signal);

        if (forecast.status === "succeeded") {
            return forecast;
        }

        if (forecast.status === "failed") {
            throw new Error(forecast.error.message || "Не удалось построить прогноз. Попробуйте ещё раз.");
        }

        await waitForForecast(pollAfter, signal);
    }

    throw new Error("Прогноз ещё рассчитывается. Повторите проверку немного позже.");
};


const getDashboardData = async ( period: ForecastPeriod, signal: AbortSignal ): Promise<IDashboardData> => {
    const companies = await companiesService.listCompanies({ limit: 1 }, signal);
    const company = companies.items[0];

    if (!company) {
        return { company: null, account: null, forecast: null };
    }

    const account = await ledgerService.getDefaultAccount({ company_id: company.id }, signal);

    if (!account.balance || !account.balance_as_of) {
        return { company, account, forecast: null };
    }

    const forecasts = await forecastsService.listForecasts({ company_id: company.id, limit: 100, include_scenarios: false }, signal);
    const existing = forecasts.items.find(( forecast ) => {
        const horizon = Math.round((Date.parse(forecast.end_date) - Date.parse(forecast.cutoff_date)) / 86400000);

        return forecast.cutoff_date === account.balance_as_of && horizon === period && !forecast.is_stale && forecast.status !== "failed";
    });
    const accepted = existing || await forecastsService.createForecast({ company_id: company.id, idempotency_key: crypto.randomUUID(), data: { cutoff_date: account.balance_as_of, horizon_days: period } }, signal);
    const forecast = await completeForecast(company.id, accepted.id, "poll_after_ms" in accepted ? accepted.poll_after_ms : 1000, signal);

    return { company, account, forecast };
};


const dashboardService = { getDashboardData };


export default dashboardService;
