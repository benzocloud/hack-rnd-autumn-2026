import type { Metadata } from "next";

import { AuthForm } from "@/features";
import type { VerificationPurpose } from "@/interfaces";


interface IProps {
    searchParams: Promise<{ purpose?: string }>;
}


const metadata: Metadata = {
    title: "Подтверждение кодом",
};


const Page = async ( { searchParams }: IProps ) => {
    const { purpose } = await searchParams;
    const verificationPurpose: VerificationPurpose = purpose === "login" || purpose === "reset" ? purpose : "register";

    return <AuthForm key={verificationPurpose} mode="verify-code" verificationPurpose={verificationPurpose} />;
};


export { metadata };
export default Page;
