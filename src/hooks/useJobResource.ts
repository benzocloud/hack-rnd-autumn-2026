"use client";

import { useEffect } from "react";

import useResource from "./useResource";


const useJobResource = <T extends { status: string },>( loader: ( signal: AbortSignal ) => Promise<T> ) => {
    const resource = useResource(loader);
    const { data, loading, error, reload } = resource;


    useEffect(() => {
        if (!data || loading || error || ![ "queued", "processing", "running" ].includes(data.status)) {
            return;
        }

        const timer = setTimeout(reload, 3000);

        return () => clearTimeout(timer);
    }, [ data, loading, error, reload ]);


    return resource;
};


export default useJobResource;
