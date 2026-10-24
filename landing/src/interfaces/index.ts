interface IForecastPoint {
    date: string;
    balance: { amount_minor: string; currency: "RUB" };
}

type DemoScenario = "base" | "delay" | "sales" | "expense" | "recurring";


export type { IForecastPoint, DemoScenario };
