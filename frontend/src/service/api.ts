import axios from "axios";

import { redirectToLogin } from "@/helpers/auth";
import getErrorMessage from "@/helpers/getErrorMessage";
import { showErrorSnackbar } from "@/helpers/snackbar";
import type { ICsrfToken } from "@/interfaces";

import mockAdapter from "./mockAdapter";
import { API_URL } from "./url";


const apiConfig = {
    baseURL: API_URL,
    withCredentials: true,
    ...(process.env.developerMode === "mock" ? { adapter: mockAdapter } : {}),
};


const api = axios.create(apiConfig);


const csrfApi = axios.create(apiConfig);


const sessionApi = axios.create(apiConfig);


const notifiedErrors = new WeakSet<object>();


let csrfToken: ICsrfToken | null = null;

let csrfRequest: Promise<ICsrfToken> | null = null;


const getCsrfToken = (): Promise<ICsrfToken> => {
    if (csrfToken && Date.parse(csrfToken.expires_at) > Date.now()) {
        return Promise.resolve(csrfToken);
    }

    if (!csrfRequest) {
        csrfRequest = csrfApi.get<ICsrfToken>("/auth/csrf")
            .then(( response ) => {
                csrfToken = response.data;

                return response.data;
            })
            .finally(() => {
                csrfRequest = null;
            });
    }

    return csrfRequest;
};


api.interceptors.request.use(async ( config ) => {
    if (process.env.developerMode === "mock") {
        return config;
    }

    if (config.method && ![ "get", "head", "options" ].includes(config.method.toLowerCase())) {
        if (typeof window === "undefined") {
            throw new Error("Изменение данных доступно только из браузера.");
        }

        const token = await getCsrfToken();

        config.headers.set("X-CSRF-Token", token.csrf_token);
    }

    return config;
});


api.interceptors.response.use(
    ( response ) => {
        if (response.config.url === "/auth/csrf") {
            csrfToken = response.data;
        } else if (response.config.url?.startsWith("/auth/")) {
            csrfToken = null;
        }

        return response;
    },
    ( error: unknown ) => {
        if (axios.isAxiosError(error) && (error.response?.status === 401 || error.response?.status === 403)) {
            csrfToken = null;
        }

        if (axios.isAxiosError(error) && error.response?.status === 401) {
            redirectToLogin();
        }

        if (!axios.isCancel(error) && error && typeof error === "object" && !notifiedErrors.has(error)) {
            notifiedErrors.add(error);
            showErrorSnackbar(getErrorMessage(error));
        }

        return Promise.reject(error);
    },
);


export { sessionApi };
export default api;
