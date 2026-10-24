"use client";

import { useSearchParams } from "next/navigation";

import type { VerificationPurpose } from "@/interfaces";

import AuthForm from "./AuthForm";


interface IProps {
    mode: "verify-code" | "reset-password";
}


const AuthQueryForm = ( { mode }: IProps ) => {
    const searchParams = useSearchParams();
    const purpose = searchParams.get("purpose");
    const verificationPurpose: VerificationPurpose = purpose === "login" || purpose === "reset" ? purpose : "register";

    return mode === "reset-password"
        ? <AuthForm key={searchParams.get("token") || ""} mode={mode} resetToken={searchParams.get("token") || ""} />
        : <AuthForm key={verificationPurpose} mode={mode} verificationPurpose={verificationPurpose} />;
};


export default AuthQueryForm;
