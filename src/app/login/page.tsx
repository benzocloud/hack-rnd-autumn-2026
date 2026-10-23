import type { Metadata } from "next";

import { AuthForm } from "@/features";


const metadata: Metadata = {
    title: "Вход",
};


const Page = () => {
    return <AuthForm mode="login" />;
};


export { metadata };
export default Page;
