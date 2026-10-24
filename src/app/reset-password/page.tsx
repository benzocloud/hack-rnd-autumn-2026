import type { Metadata } from "next";

import { AuthForm } from "@/features";


interface IProps {
    searchParams: Promise<{ token?: string }>;
}


const metadata: Metadata = {
    title: "Новый пароль",
};


const Page = async ( { searchParams }: IProps ) => {
    const { token } = await searchParams;

    return <AuthForm mode="reset-password" resetToken={token || ""} />;
};


export { metadata };
export default Page;
