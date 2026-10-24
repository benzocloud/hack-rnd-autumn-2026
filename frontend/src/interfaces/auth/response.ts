import type { IUser } from "@/interfaces";


interface IPersonalDataConsentDocument {
    document_version: string;
    document_sha256: string;
    text: string;
}


interface IAcknowledgement {
    message: string;
}


interface ISession {
    user: IUser;
    expires_at: string;
}


interface ICsrfToken {
    csrf_token: string;
    expires_at: string;
}


export type { IPersonalDataConsentDocument, IAcknowledgement, ISession, ICsrfToken };
