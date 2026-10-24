const formatMoney = ( amountMinor: string ): string => {
    const amount = BigInt(amountMinor);
    const negative = amount < BigInt(0);
    const absolute = negative ? -amount : amount;
    const rubles = new Intl.NumberFormat("ru-RU").format(absolute / BigInt(100));
    const kopecks = absolute % BigInt(100);

    return `${negative ? "−" : ""}${rubles}${kopecks ? `,${String(kopecks).padStart(2, "0")}` : ""} руб.`;
};


export default formatMoney;
