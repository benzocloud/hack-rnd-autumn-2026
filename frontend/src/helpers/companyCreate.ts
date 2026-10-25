const storageName = ( scope: string ) => `saldo-company-create:${scope}`;
const fallbackKeys = new Map<string, string>();


const rememberCompanyCreateKey = ( store: Pick<Storage, "getItem" | "setItem">, scope: string ) => {
    const name = storageName(scope);
    const existing = store.getItem(name);

    if (existing) {
        return existing;
    }

    const key = crypto.randomUUID();

    store.setItem(name, key);

    return key;
};


const fallbackKey = ( scope: string ) => {
    const existing = fallbackKeys.get(scope);

    if (existing) {
        return existing;
    }

    const key = crypto.randomUUID();

    fallbackKeys.set(scope, key);

    return key;
};


const companyCreateKeyFrom = ( store: Pick<Storage, "getItem" | "setItem">, scope: string ) => {
    try {
        return rememberCompanyCreateKey(store, scope);
    } catch {
        return fallbackKey(scope);
    }
};


const companyCreateKey = ( scope: string ) => companyCreateKeyFrom(sessionStorage, scope);


const clearCompanyCreateKey = ( scope: string ) => {
    fallbackKeys.delete(scope);

    try {
        sessionStorage.removeItem(storageName(scope));
    } catch {
        // A private browser can refuse storage. The in-memory key is already dropped.
    }
};


export { rememberCompanyCreateKey, companyCreateKeyFrom, companyCreateKey, clearCompanyCreateKey };
