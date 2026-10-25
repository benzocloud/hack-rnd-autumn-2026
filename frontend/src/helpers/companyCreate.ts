const storageName = ( scope: string ) => `saldo-company-create:${scope}`;


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


const companyCreateKey = ( scope: string ) => {
    try {
        return rememberCompanyCreateKey(sessionStorage, scope);
    } catch {
        return crypto.randomUUID();
    }
};


const clearCompanyCreateKey = ( scope: string ) => {
    try {
        sessionStorage.removeItem(storageName(scope));
    } catch {
        // A private browser can refuse storage. The successful response is already in hand.
    }
};


export { rememberCompanyCreateKey, companyCreateKey, clearCompanyCreateKey };
