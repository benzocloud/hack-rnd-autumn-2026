"use client";

import axios from "axios";
import { useCallback, useEffect, useRef, useState } from "react";

import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar, showSuccessSnackbar } from "@/helpers/snackbar";


const useRequest = () => {
    const [ busy, setBusy ] = useState(false);
    const [ error, setError ] = useState<string | null>(null);
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

                if (!axios.isAxiosError(cause)) {
                    showErrorSnackbar(getErrorMessage(cause));
                }

                if (cause && typeof cause === "object" && "field" in cause && typeof cause.field === "string" && cause.field) {
                    document.querySelector<HTMLElement>(`[name="${CSS.escape(cause.field)}"]`)?.focus();
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


    return { run, busy, error };
};


export default useRequest;
