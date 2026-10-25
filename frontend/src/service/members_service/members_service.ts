import axios from "axios";

import type { IAcceptInvitationParams, ICompany, ICreateInvitationParams, IDeclineInvitationParams, IInvitation, IInvitationList, IListCompanyInvitationsParams, IListMembersParams, IListMyInvitationsParams, IMembershipList, IRemoveMemberParams, IRevokeInvitationParams } from "@/interfaces";

import api from "../api";


const listMembers = async ( request: IListMembersParams, signal?: AbortSignal ): Promise<IMembershipList> => {
    const response = await api.request<IMembershipList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/members`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const removeMember = async ( request: IRemoveMemberParams, signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "DELETE",
        url: `/companies/${encodeURIComponent(request.company_id)}/members/${encodeURIComponent(request.user_id)}`,
        signal,
    });


    return response.data;
};


const listCompanyInvitations = async ( request: IListCompanyInvitationsParams, signal?: AbortSignal ): Promise<IInvitationList> => {
    const response = await api.request<IInvitationList>({
        method: "GET",
        url: `/companies/${encodeURIComponent(request.company_id)}/invitations`,
        signal,
        params: {
            limit: request.limit,
            cursor: request.cursor,
        },
    });


    return response.data;
};


const createInvitation = async ( request: ICreateInvitationParams, signal?: AbortSignal ): Promise<IInvitation> => {
    const response = await api.request<IInvitation>({
        method: "POST",
        url: `/companies/${encodeURIComponent(request.company_id)}/invitations`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const revokeInvitation = async ( request: IRevokeInvitationParams, signal?: AbortSignal ): Promise<void> => {
    const response = await api.request<void>({
        method: "DELETE",
        url: `/companies/${encodeURIComponent(request.company_id)}/invitations/${encodeURIComponent(request.invitation_id)}`,
        signal,
    });


    return response.data;
};


const listMyInvitations = async ( request: IListMyInvitationsParams = {}, signal?: AbortSignal ): Promise<IInvitationList> => {
    try {
        const response = await api.request<IInvitationList>({
            method: "GET",
            url: "/me/invitations",
            signal,
            params: {
                limit: request.limit,
                cursor: request.cursor,
            },
        });

        return response.data;
    } catch (error: unknown) {
        // Invitations are still optional on some deployments; missing route must not block Workspace.
        if (axios.isAxiosError(error) && error.response?.status === 404) {
            return { items: [], next_cursor: null };
        }

        throw error;
    }
};


const acceptInvitation = async ( request: IAcceptInvitationParams, signal?: AbortSignal ): Promise<ICompany> => {
    const response = await api.request<ICompany>({
        method: "POST",
        url: `/me/invitations/${encodeURIComponent(request.invitation_id)}/accept`,
        signal,
        data: request.data,
        headers: { "Idempotency-Key": request.idempotency_key },
    });


    return response.data;
};


const declineInvitation = async ( request: IDeclineInvitationParams, signal?: AbortSignal ): Promise<IInvitation> => {
    const response = await api.request<IInvitation>({
        method: "POST",
        url: `/me/invitations/${encodeURIComponent(request.invitation_id)}/decline`,
        signal,
        data: request.data,
    });


    return response.data;
};


const membersService = { listMembers, removeMember, listCompanyInvitations, createInvitation, revokeInvitation, listMyInvitations, acceptInvitation, declineInvitation };


export default membersService;
