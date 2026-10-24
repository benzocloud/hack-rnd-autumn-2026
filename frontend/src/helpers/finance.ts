const formatDate = ( value: string | null | undefined, withYear = true ): string => {
    if (!value) {
        return "Не указана";
    }

    const date = new Date(`${value.slice(0, 10)}T00:00:00Z`);

    return Number.isNaN(date.getTime()) ? "Не указана" : new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", ...(withYear ? { year: "numeric" } : {}), timeZone: "UTC" }).format(date);
};


const today = () => new Intl.DateTimeFormat("sv-SE", { timeZone: "Europe/Moscow" }).format(new Date());


const addDays = ( date: string, days: number ) => new Date(Date.parse(`${date}T00:00:00Z`) + days * 86400000).toISOString().slice(0, 10);


const validDate = ( value: string ): boolean => /^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(Date.parse(value)) && new Date(`${value}T00:00:00Z`).toISOString().slice(0, 10) === value;


const invalid: ( message: string, field?: string ) => never = ( message, field = "" ) => {
    throw Object.assign(new Error(message), { field });
};


const textField = ( form: FormData, name: string, label: string, maximum = 200, optional = false ): string => {
    const value = String(form.get(name) || "").trim();

    if (!value && !optional) {
        invalid(`Заполните поле «${label}».`, name);
    }

    if (value.length > maximum) {
        invalid(`В поле «${label}» допускается не больше ${maximum} символов.`, name);
    }

    return value;
};


const dateField = ( form: FormData, name: string, label: string ): string => {
    const value = textField(form, name, label, 10);

    if (!validDate(value)) {
        invalid(`Укажите корректную дату в поле «${label}».`, name);
    }

    return value;
};


const parseMoney = ( value: string, allowNegative = false ): string => {
    const normalized = value.trim().replace(/[\s\u00a0]/g, "").replace(",", ".");

    if (!/^-?\d{1,16}(\.\d{1,2})?$/.test(normalized) || (!allowNegative && normalized.startsWith("-"))) {
        invalid("Укажите сумму в рублях, не больше двух знаков после запятой.", "amount");
    }

    const [ whole, fraction = "" ] = normalized.replace("-", "").split(".");
    const amount = (BigInt(whole) * BigInt(100) + BigInt(fraction.padEnd(2, "0"))) * (normalized.startsWith("-") ? BigInt(-1) : BigInt(1));

    if (!allowNegative && amount <= BigInt(0)) {
        invalid("Сумма должна быть больше нуля.", "amount");
    }

    return String(amount);
};


const moneyInput = ( minor: string ): string => {
    const amount = BigInt(minor);
    const positive = amount < BigInt(0) ? -amount : amount;

    return `${amount < BigInt(0) ? "-" : ""}${positive / BigInt(100)},${String(positive % BigInt(100)).padStart(2, "0")}`;
};


const companyPath = ( companyId: string, page = "dashboard" ) => `/companies/${encodeURIComponent(companyId)}/${page}`;


const formIdentity = ( form: FormData ) => JSON.stringify(Array.from(form.entries()).map(( [ key, value ] ) => [ key, typeof value === "string" ? value : `${value.name}:${value.size}:${value.lastModified}` ]));


const sourceName = ( kind: string ) => ({ manual: "Вручную", file: "Из файла", mock_external: "Демо-источник" }[kind] || "Источник не указан");


export { formatDate, today, addDays, validDate, invalid, textField, dateField, parseMoney, moneyInput, companyPath, formIdentity, sourceName };
