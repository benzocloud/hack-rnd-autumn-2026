import type { DemoScenario, IForecastPoint } from "@/interfaces";


const anchors = [ 680000, 640000, 710000, 520000, 565000, 390000, 430000, 210000, 290000, 120000, -42000, 175000, 240000, 220000, 340000, 395000, 460000, 380000, 490000 ];

const demoDate = ( day: number ): string => new Date(Date.UTC(2026, 9, 24 + day)).toISOString().slice(0, 10);


const makeDemoForecast = ( period: number, scenario: DemoScenario = "base" ): IForecastPoint[] => {
    return Array.from({ length: period }, ( _, day ) => {
        const position = Math.floor(day / 2);
        const left = BigInt(anchors[position % anchors.length]) * BigInt(100);
        const right = BigInt(anchors[(position + 1) % anchors.length]) * BigInt(100);
        let amount = left + (right - left) * BigInt(day % 2) / BigInt(2);

        if (scenario === "delay" && day >= 12 && day < 23) {
            amount -= BigInt(18000000);
        }
        if (scenario === "sales" && day > 5) {
            amount -= BigInt(day - 5) * BigInt(650000);
        }
        if (scenario === "expense" && day >= 10) {
            amount -= BigInt(25000000);
        }
        if (scenario === "recurring" && day >= 7) {
            amount -= BigInt(Math.floor(day / 7)) * BigInt(4500000);
        }

        return { date: demoDate(day), balance: { amount_minor: amount.toString(), currency: "RUB" } };
    });
};


export { makeDemoForecast, demoDate };
