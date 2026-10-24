import { AxiosError, CanceledError, type AxiosAdapter, type InternalAxiosRequestConfig } from "axios";

import type { IMockResponse, IVersionRequest } from "@/interfaces";
import handleMockAuth from "@/mock/auth";
import authResponses from "@/mock/authResponses.json";
import handleMockForecasts from "@/mock/forecasts";
import { checkVersion, findItem, isMockError, mockError, nextVersion, now, paginate } from "@/mock/helpers";
import handleMockResources from "@/mock/resources";
import responses from "@/mock/responses.json";
import { readMockState, writeMockState, type IMockState } from "@/mock/state";


const mockResponses: IMockResponse[] = [ ...responses, ...authResponses ];

const statusTexts: Record<number, string> = { 200: "OK", 201: "Created", 202: "Accepted", 204: "No Content" };


const waitForMock = ( config: InternalAxiosRequestConfig ) => new Promise<void>(( resolve, reject ) => {
    const finish = () => {
        config.signal?.removeEventListener?.("abort", abort);
        resolve();
    };
    const abort = () => {
        clearTimeout(timer);
        config.signal?.removeEventListener?.("abort", abort);
        reject(new CanceledError("Запрос отменён.", config));
    };
    const timer = setTimeout(finish, typeof window === "undefined" ? 0 : 300);

    config.signal?.addEventListener?.("abort", abort);

    if (config.signal?.aborted) {
        abort();
    }
});


const handleInvitations = ( segments: string[], method: string, data: unknown, params: Record<string, unknown>, state: IMockState ) => {
    const invitations = Object.values(state.companyData).flatMap(( company ) => company.invitations).filter(( item ) => item.email === state.user?.email);

    if (method === "GET") {
        return paginate(invitations, params);
    }

    const invitation = findItem(invitations, segments[2]);

    checkVersion(invitation.version, (data as IVersionRequest).version);
    invitation.status = segments[3] === "accept" ? "accepted" : "declined";
    invitation.version = nextVersion(invitation.version);

    const company = findItem(state.companies, invitation.company_id);
    const members = state.companyData[company.id].members;

    if (invitation.status === "accepted" && state.user && !members.some(( item ) => item.user_id === state.user?.id)) {
        members.push({ user_id: state.user.id, email: state.user.email, display_name: state.user.display_name, role: "financier", joined_at: now() });
    }

    return invitation.status === "accepted" ? company : invitation;
};


const mockAdapter: AxiosAdapter = async ( config ) => {
    await waitForMock(config);
    config.cancelToken?.throwIfRequested();

    const method = config.method?.toUpperCase() || "GET";
    const url = new URL(config.url || "/", "http://mock.local");
    const path = url.pathname.replace(/^\/api\/v1(?=\/)/, "");
    const segments = path.split("/").filter(Boolean).map(decodeURIComponent);
    const params: Record<string, unknown> = { ...Object.fromEntries(url.searchParams), ...config.params };
    const response = mockResponses.find(( item ) => {
        const expected = item.path.split("/").filter(Boolean);

        return item.method === method && expected.length === segments.length && expected.every(( segment, index ) => segment.startsWith("{") || segment === segments[index]);
    });

    try {
        if (!response) {
            throw mockError(404, "Мок-ответ для этого запроса не найден.", "NOT_FOUND");
        }

        const state = readMockState();
        const data: unknown = typeof config.data === "string" ? JSON.parse(config.data) : config.data;
        const key = config.headers.get("Idempotency-Key");
        const cacheKey = key ? `${state.user?.id || "public"}:${method}:${path}:${key}` : "";
        let result: unknown;

        if (segments[0] === "auth" || path === "/me") {
            result = handleMockAuth(path, data, state);
        } else {
            handleMockAuth("/me", undefined, state);

            if (cacheKey && state.idempotency[cacheKey]) {
                return { ...structuredClone(state.idempotency[cacheKey]), statusText: statusTexts[response.status], headers: { "Content-Type": "application/json" }, config };
            }

            if (segments[0] === "me" && segments[1] === "invitations") {
                result = handleInvitations(segments, method, data, params, state);
            } else if (segments[2] === "forecasts" || segments[2] === "scenarios") {
                findItem(state.companies, segments[1]);
                result = handleMockForecasts(method, segments, data, params, state.companyData[segments[1]]);
            } else {
                result = handleMockResources(method, segments, data, params, state);
            }
        }

        if (method !== "GET") {
            if (cacheKey) {
                state.idempotency[cacheKey] = { status: response.status, data: structuredClone(result) };
                state.idempotency = Object.fromEntries(Object.entries(state.idempotency).slice(-100));
            }

            writeMockState(state);
        }

        return { data: structuredClone(result), status: response.status, statusText: statusTexts[response.status], headers: response.body ? { "Content-Type": "application/json" } : {}, config };
    } catch (error: unknown) {
        if (!isMockError(error)) {
            throw error;
        }

        const result = { data: { error: { code: error.code, message: error.message, field_errors: [], request_id: "mock" } }, status: error.status, statusText: "Mock Error", headers: { "Content-Type": "application/json" }, config };

        throw new AxiosError(error.message, error.status >= 500 ? AxiosError.ERR_BAD_RESPONSE : AxiosError.ERR_BAD_REQUEST, config, undefined, result);
    }
};


export default mockAdapter;
