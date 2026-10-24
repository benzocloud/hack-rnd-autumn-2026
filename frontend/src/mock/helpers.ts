import type { IMockCompanyData } from "./state";


interface IMockError extends Error {
    status: number;
    code: string;
}


const mockError = ( status: number, message: string, code = "MOCK_ERROR" ): IMockError => {
    return Object.assign(new Error(message), { name: "MockError", status, code });
};


const isMockError = ( error: unknown ): error is IMockError => {
    return error instanceof Error && error.name === "MockError" && "status" in error && typeof error.status === "number" && "code" in error && typeof error.code === "string";
};


const mockId = ( prefix: string ) => `${prefix}_${crypto.randomUUID()}`;


const now = () => new Date().toISOString();


const expiresAt = () => new Date(Date.now() + 86400000).toISOString();


const findItem = <T extends { id: string },>( items: T[], id: string ): T => {
    const item = items.find(( candidate ) => candidate.id === id);

    if (!item) {
        throw mockError(404, "Данные не найдены.", "NOT_FOUND");
    }

    return item;
};


const nextVersion = ( version: string ) => String(BigInt(version) + BigInt(1));


const checkVersion = ( current: string, expected?: string ) => {
    if (expected !== undefined && current !== expected) {
        throw mockError(409, "Данные изменились. Обновите страницу и повторите действие.", "VERSION_CONFLICT");
    }
};


const touchData = ( company: IMockCompanyData ) => {
    company.account.data_version = nextVersion(company.account.data_version);
    company.forecasts.forEach(( forecast ) => {
        forecast.current_data_version = company.account.data_version;
        forecast.is_stale = true;
    });
};


const paginate = <T,>( items: T[], params: Record<string, unknown> ) => {
    const offset = Math.max(0, Number(params.cursor) || 0);
    const limit = Math.min(100, Math.max(1, Number(params.limit) || 50));

    return {
        items: items.slice(offset, offset + limit),
        next_cursor: offset + limit < items.length ? String(offset + limit) : null,
    };
};


const inPeriod = ( date: string, params: Record<string, unknown> ) => {
    return (!params.from || date >= String(params.from)) && (!params.to || date <= String(params.to));
};


export { mockError, isMockError, mockId, now, expiresAt, findItem, nextVersion, checkVersion, touchData, paginate, inPeriod };
