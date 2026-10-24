import type { IUser } from "@/interfaces";


interface IPersonalDataConsentDocument {
    document_version: string;
    document_sha256: string;
    text: string;
}


interface IAcknowledgement {
    message: string;
}


interface IEmailVerificationChallenge extends IAcknowledgement {
    challenge_id: string;
    expires_at: string;
}


interface ISession {
    user: IUser;
    expires_at: string;
}


interface ICsrfToken {
    email_verification_mode?: "strict" | "demo";
    csrf_token: string;
    expires_at: string;
}


export type { IPersonalDataConsentDocument, IAcknowledgement, IEmailVerificationChallenge, ISession, ICsrfToken };
