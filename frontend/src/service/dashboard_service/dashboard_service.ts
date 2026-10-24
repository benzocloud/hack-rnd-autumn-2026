import { CanceledError } from "axios";

import { collectPages } from "@/helpers/pagination";
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
    const timer = setTimeout(finish, Math.max(1000, duration));

    signal.addEventListener("abort", abort, { once: true });

    if (signal.aborted) {
        abort();
    }
});


const completeForecast = async ( companyId: string, forecastId: string, pollAfter: number, signal: AbortSignal, onStatus?: ( status: IForecast["status"] ) => void ): Promise<IForecastSucceeded> => {
    for (let attempt = 0; attempt < 30; attempt++) {
        const forecast: IForecast = await forecastsService.getForecast({ company_id: companyId, forecast_id: forecastId }, signal);
        onStatus?.(forecast.status);

        if (forecast.status === "succeeded") {
            const points = forecast.result.daily_points;
            const start = Date.parse(`${forecast.input.start_date}T00:00:00Z`);

            if (forecast.input.horizon_days < 30 || points.length !== forecast.input.horizon_days || points.some(( point, index ) => Date.parse(`${point.date}T00:00:00Z`) !== start + index * 86400000)) {
                throw new Error("API вернул неполный дневной прогноз. Пропуски не заменены нулями. Повторите загрузку.");
            }

            return forecast;
        }

        if (forecast.status === "failed") {
            throw new Error(forecast.error.message || "Не удалось построить прогноз. Попробуйте ещё раз.");
        }

        await waitForForecast(pollAfter, signal);
    }

    throw new Error("Прогноз ещё рассчитывается. Повторите проверку немного позже.");
};


const getDashboardData = async ( period: ForecastPeriod, signal: AbortSignal, companyId?: string, onStatus?: ( status: IForecast["status"] ) => void ): Promise<IDashboardData> => {
    const company = companyId ? await companiesService.getCompany({ company_id: companyId }, signal) : (await companiesService.listCompanies({ limit: 1 }, signal)).items[0];

    if (!company) {
        return { company: null, account: null, forecast: null };
    }

    const account = await ledgerService.getDefaultAccount({ company_id: company.id }, signal);

    if (!account.balance || !account.balance_as_of) {
        return { company, account, forecast: null };
    }

    const forecasts = await collectPages(( cursor ) => forecastsService.listForecasts({ company_id: company.id, limit: 100, cursor, include_scenarios: false }, signal));
    const existing = forecasts.find(( forecast ) => {
        const horizon = Math.round((Date.parse(forecast.end_date) - Date.parse(forecast.cutoff_date)) / 86400000);

        return forecast.cutoff_date === account.balance_as_of && horizon === period && !forecast.is_stale && forecast.status !== "failed";
    });
    const accepted = existing || await forecastsService.createForecast({ company_id: company.id, idempotency_key: crypto.randomUUID(), data: { cutoff_date: account.balance_as_of, horizon_days: period } }, signal);
    const forecast = await completeForecast(company.id, accepted.id, "poll_after_ms" in accepted ? accepted.poll_after_ms : 1000, signal, onStatus);

    return { company, account, forecast };
};


const dashboardService = { getDashboardData };


export default dashboardService;
