import type { Metadata } from "next";

import { AuthForm } from "@/features";


const metadata: Metadata = {
    title: "Восстановление пароля",
};


const Page = () => {
    return <AuthForm mode="forgot-password" />;
};


export { metadata };
export default Page;
