import type { IUser } from "@/interfaces";

import api, { sessionApi } from "../api";


const getMe = async ( signal?: AbortSignal ): Promise<IUser> => {
    const response = await api.request<IUser>({
        method: "GET",
        url: "/me",
        signal,
    });


    return response.data;
};


const checkSession = async ( signal?: AbortSignal ): Promise<IUser> => {
    const response = await sessionApi.get<IUser>("/me", { signal });

    return response.data;
};


const accountService = { getMe, checkSession };


export default accountService;
