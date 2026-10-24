import type { IUser } from "@/interfaces/Entities";
import type { IEmailVerificationChallenge } from "./response";


type AuthMode = "login" | "register" | "verify-code" | "forgot-password" | "reset-password";


type VerificationPurpose = "register" | "login" | "reset";


interface IAuthState {
    user: IUser | null;
    status: "idle" | "checking" | "ready" | "failed";
    verificationEmail: string;
    verificationChallenge: IEmailVerificationChallenge | null;
}


export type { AuthMode, VerificationPurpose, IAuthState };
