import type { Metadata } from "next";

import { AuthForm } from "@/features";


const metadata: Metadata = {
    title: "Регистрация",
};


const Page = () => {
    return <AuthForm mode="register" />;
};


export { metadata };
export default Page;
