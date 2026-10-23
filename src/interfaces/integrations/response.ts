interface IExternalProvider {
    id: string;
    name: string;
    mode: "mock";
    capabilities: Array<"transactions" | "planned_payments" | "balance_observations">;
    adapter: string;
}


interface IConnection {
    id: string;
    company_id: string;
    account_id: string;
    provider_id: string;
    mode: "mock";
    display_name: string;
    source_company_id: string;
    status: "ready" | "disabled";
    created_at: string;
    version: string;
}


interface IExternalProviderList {
    items: Array<IExternalProvider>;
    next_cursor: string | null;
}


interface IConnectionList {
    items: Array<IConnection>;
    next_cursor: string | null;
}


export type { IExternalProvider, IConnection, IExternalProviderList, IConnectionList };
