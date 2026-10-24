"use client";

import axios from "axios";
import { useCallback, useEffect, useRef, useState } from "react";

import getErrorMessage from "@/helpers/getErrorMessage";
import { getFieldErrors, type FieldErrors } from "@/helpers/fieldErrors";
import { showErrorSnackbar, showSuccessSnackbar } from "@/helpers/snackbar";


const useRequest = () => {
    const [ busy, setBusy ] = useState(false);
    const [ error, setError ] = useState<string | null>(null);
    const [ fieldErrors, setFieldErrors ] = useState<FieldErrors>({});
    const controller = useRef<AbortController | null>(null);
    const action = useRef<{ identity: string; key: string } | null>(null);


    useEffect(() => () => controller.current?.abort(), []);


    const run = useCallback(async <T,>( task: ( signal: AbortSignal, key: string ) => Promise<T>, message = "", identity = "action" ): Promise<{ ok: true; data: T } | { ok: false }> => {
        if (controller.current) {
            return { ok: false };
        }

        const request = new AbortController();

        controller.current = request;
        action.current = action.current?.identity === identity ? action.current : { identity, key: crypto.randomUUID() };
        setBusy(true);
        setError(null);
        setFieldErrors({});

        try {
            const data = await task(request.signal, action.current.key);

            if (request.signal.aborted) {
                return { ok: false };
            }

            action.current = null;

            if (message) {
                showSuccessSnackbar(message);
            }

            return { ok: true, data };
        } catch (cause: unknown) {
            if (!request.signal.aborted && !axios.isCancel(cause)) {
                setError(getErrorMessage(cause));
                const fields = getFieldErrors(cause);
                setFieldErrors(fields);

                if (!axios.isAxiosError(cause)) {
                    showErrorSnackbar(getErrorMessage(cause));
                }

            }

            return { ok: false };
        } finally {
            if (!request.signal.aborted) {
                setBusy(false);
            }

            controller.current = null;
        }
    }, []);


    const clearFieldError = ( name?: string ) => setFieldErrors(( current ) => name ? Object.fromEntries(Object.entries(current).filter(( [ field ] ) => field !== name)) : {});


    return { run, busy, error, fieldErrors, clearFieldError };
};


export default useRequest;
