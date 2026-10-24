type EmailVerificationMode = "strict" | "demo";


// This response metadata labels the UI only; it is never sent as a mode selector.
const getEmailVerificationMode = ( response: unknown ): EmailVerificationMode | null => {
    if (typeof response === "object" && response !== null && "email_verification_mode" in response && response.email_verification_mode === "demo") {
        return "demo";
    }

    if (typeof response === "object" && response !== null && "email_verification_mode" in response && response.email_verification_mode === "strict") {
        return "strict";
    }

    return null;
};


export { getEmailVerificationMode };
export type { EmailVerificationMode };
