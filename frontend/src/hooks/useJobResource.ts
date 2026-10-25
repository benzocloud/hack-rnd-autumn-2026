"use client";

import { useCallback, useEffect, useState } from "react";

import useResource from "./useResource";


const useJobResource = <T extends { status: string },>( loader: ( signal: AbortSignal ) => Promise<T> ) => {
    const resource = useResource(loader);
    const { data, loading, error, reload } = resource;
    const [ polling, setPolling ] = useState({ loader, attempts: 0 });
    const attempts = polling.loader === loader ? polling.attempts : 0;
    const pending = !!data && [ "queued", "processing", "running" ].includes(data.status);
    const timedOut = pending && attempts >= 40;
    const retry = useCallback(() => {
        setPolling({ loader, attempts: 0 });
        reload();
    }, [ loader, reload ]);


    useEffect(() => {
        if (!pending || loading || error || timedOut) { return; }
        const timer = setTimeout(() => {
            setPolling({ loader, attempts: attempts + 1 });
            reload();
        }, 3000);
        return () => clearTimeout(timer);
    }, [ pending, loading, error, timedOut, attempts, loader, reload ]);


    return { ...resource, reload: retry, error: error || (timedOut ? "Автоматическая проверка приостановлена. Задание продолжает выполняться на сервере; повторите проверку статуса позже." : null) };
};


export default useJobResource;
