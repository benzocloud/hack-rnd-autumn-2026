import type { IUser } from "@/interfaces/Entities";


type AuthMode = "login" | "register" | "verify-code" | "forgot-password" | "reset-password";


type VerificationPurpose = "register" | "login" | "reset";


interface IAuthState {
    user: IUser | null;
    status: "idle" | "checking" | "ready" | "failed";
    verificationEmail: string;
}


export type { AuthMode, VerificationPurpose, IAuthState };
