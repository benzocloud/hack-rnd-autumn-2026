import type { IAccount, ICompany, IConnection, IEmailVerificationChallenge, IForecastSucceeded, IImport, IInvitation, IMatch, IMembership, IPayment, IScenario, ITransaction, IUser } from "@/interfaces";

import bodies from "./bodies.json";


interface IMockCompanyData {
    account: IAccount;
    members: IMembership[];
    invitations: IInvitation[];
    transactions: ITransaction[];
    payments: IPayment[];
    matches: IMatch[];
    connections: IConnection[];
    imports: IImport[];
    forecasts: IForecastSucceeded[];
    scenarios: IScenario[];
}


interface IMockState {
    schema: 2;
    user: IUser | null;
    registeredUsers: IUser[];
    pendingEmail: string;
    verificationChallenge: (IEmailVerificationChallenge & { consumedCode?: string }) | null;
    pendingLoginUser: IUser | null;
    resetEmail: string;
    passwordResetVerified: boolean;
    expiresAt: string | null;
    companies: ICompany[];
    companyData: Record<string, IMockCompanyData>;
    idempotency: Record<string, { status: number; data: unknown }>;
}


const STORAGE_KEY = "saldo-mock-state-v2";


const fixture = <T,>( name: keyof typeof bodies ): T => structuredClone(bodies[name]) as T;


const createCompanyData = ( company: ICompany, seeded = false ): IMockCompanyData => {
    const account = fixture<IAccount>("Account");

    account.id = company.default_account_id;
    account.company_id = company.id;

    if (!seeded) {
        account.balance = { amount_minor: "0", currency: "RUB" };
        account.balance_as_of = new Date().toISOString().slice(0, 10);
        account.data_version = "1";
    }

    return {
        account,
        members: seeded ? fixture<{ items: IMembership[] }>("MembershipList").items : [],
        invitations: seeded ? [ fixture<IInvitation>("Invitation") ] : [],
        transactions: seeded ? [ fixture<ITransaction>("Transaction") ] : [],
        payments: seeded ? [ fixture<IPayment>("Payment") ] : [],
        matches: [],
        connections: seeded ? [ fixture<IConnection>("Connection") ] : [],
        imports: seeded ? [ fixture<IImport>("Import") ] : [],
        forecasts: seeded ? [ fixture<IForecastSucceeded>("ForecastSucceeded") ] : [],
        scenarios: seeded ? [ fixture<IScenario>("Scenario") ] : [],
    };
};


const createState = (): IMockState => {
    const company = fixture<ICompany>("Company");
    const companyData = createCompanyData(company, true);
    const forecast = fixture<IForecastSucceeded>("ForecastSucceeded");

    forecast.id = fixture<IScenario>("Scenario").forecast_id;
    forecast.scenario_id = fixture<IScenario>("Scenario").id;
    companyData.forecasts.push(forecast);

    for (let index = 0; index < 18; index++) {
        const payment = fixture<IPayment>("Payment");
        const inflow = index % 3 === 0;
        const amount = String(inflow ? 1800000 + index * 20000 : 450000 + index * 15000);
        const date = new Date(`${companyData.account.balance_as_of}T00:00:00Z`);

        date.setUTCDate(date.getUTCDate() + 2 + index * 5);
        payment.id = crypto.randomUUID();
        payment.direction = inflow ? "inflow" : "outflow";
        payment.amount.amount_minor = amount;
        payment.remaining_amount.amount_minor = amount;
        payment.scheduled_date = date.toISOString().slice(0, 10);
        payment.counterparty_name = inflow ? "Демо-покупатель" : "Демо-поставщик";
        payment.description = inflow ? "Оплата заказа" : "Плановые расходы";
        payment.category_id = inflow ? null : fixture<{ items: { id: string }[] }>("CategoryList").items[0].id;
        companyData.payments.push(payment);
    }


    return {
        schema: 2,
        user: null,
        registeredUsers: [ fixture<IUser>("User") ],
        pendingEmail: "",
        verificationChallenge: null,
        pendingLoginUser: null,
        resetEmail: "",
        passwordResetVerified: false,
        expiresAt: null,
        companies: [ company ],
        companyData: { [company.id]: companyData },
        idempotency: {},
    };
};


let memoryState: IMockState | null = null;


const readMockState = (): IMockState => {
    if (typeof window === "undefined") {
        return createState();
    }

    try {
        const stored = window.localStorage.getItem(STORAGE_KEY);

        if (stored) {
            const parsed: IMockState = JSON.parse(stored);

            if (parsed.schema === 2 && Array.isArray(parsed.companies) && parsed.companyData && Array.isArray(parsed.registeredUsers) && parsed.idempotency) {
                memoryState = { ...parsed, pendingLoginUser: parsed.pendingLoginUser || null, resetEmail: parsed.resetEmail || "", passwordResetVerified: parsed.passwordResetVerified === true };
            }
        }
    } catch {}

    memoryState ??= createState();

    return structuredClone(memoryState);
};


const writeMockState = ( state: IMockState ) => {
    if (typeof window === "undefined") {
        return;
    }

    memoryState = structuredClone(state);

    try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {}
};


export { fixture, createCompanyData, readMockState, writeMockState };
export type { IMockState, IMockCompanyData };
