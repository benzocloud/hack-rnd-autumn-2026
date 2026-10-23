interface ICreateInvitationRequest {
    email: string;
}


interface IVersionRequest {
    version: string;
}


interface IListMembersParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface IRemoveMemberParams {
    company_id: string;
    user_id: string;
}


interface IListCompanyInvitationsParams {
    company_id: string;
    limit?: number;
    cursor?: string;
}


interface ICreateInvitationParams {
    company_id: string;
    idempotency_key: string;
    data: ICreateInvitationRequest;
}


interface IRevokeInvitationParams {
    company_id: string;
    invitation_id: string;
}


interface IListMyInvitationsParams {
    limit?: number;
    cursor?: string;
}


interface IAcceptInvitationParams {
    invitation_id: string;
    idempotency_key: string;
    data: IVersionRequest;
}


interface IDeclineInvitationParams {
    invitation_id: string;
    data: IVersionRequest;
}


export type { ICreateInvitationRequest, IVersionRequest, IListMembersParams, IRemoveMemberParams, IListCompanyInvitationsParams, ICreateInvitationParams, IRevokeInvitationParams, IListMyInvitationsParams, IAcceptInvitationParams, IDeclineInvitationParams };
