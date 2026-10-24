interface IMembership {
    user_id: string;
    email: string;
    display_name: string;
    role: "owner" | "financier";
    joined_at: string;
}


interface IInvitation {
    id: string;
    company_id: string;
    company_name: string;
    email: string;
    role: "financier";
    status: "pending" | "accepted" | "declined" | "revoked" | "expired";
    created_at: string;
    expires_at: string;
    version: string;
}


interface IMembershipList {
    items: Array<IMembership>;
    next_cursor: string | null;
}


interface IInvitationList {
    items: Array<IInvitation>;
    next_cursor: string | null;
}


export type { IMembership, IInvitation, IMembershipList, IInvitationList };
