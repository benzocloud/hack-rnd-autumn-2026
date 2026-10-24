"use client";

import axios from "axios";
import { useCallback, useEffect, useState } from "react";

import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar } from "@/helpers/snackbar";


type Loader<T> = ( signal: AbortSignal ) => Promise<T>;


const useResource = <T,>( loader: Loader<T> ) => {
    const [ revision, setRevision ] = useState(0);
    const [ result, setResult ] = useState<{ loader: Loader<T>; revision: number; data: T | null; error: string | null } | null>(null);


    useEffect(() => {
        const controller = new AbortController();

        loader(controller.signal).then(( data ) => {
            if (!controller.signal.aborted) {
                setResult({ loader, revision, data, error: null });
            }
        }).catch(( error: unknown ) => {
            if (controller.signal.aborted || axios.isCancel(error)) {
                return;
            }

            if (!axios.isAxiosError(error)) {
                showErrorSnackbar(getErrorMessage(error));
            }

            setResult(( current ) => ({ loader, revision, data: current?.loader === loader ? current.data : null, error: getErrorMessage(error) }));
        });

        return () => controller.abort();
    }, [ loader, revision ]);


    const reload = useCallback(() => setRevision(( value ) => value + 1), []);
    const current = result?.loader === loader;

    return { data: current ? result.data : null, loading: !current || result.revision !== revision, error: current && result.revision === revision ? result.error : null, reload };
};


export default useResource;
