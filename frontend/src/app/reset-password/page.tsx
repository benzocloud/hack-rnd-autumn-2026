import type { Metadata } from "next";
import { Suspense } from "react";

import AuthQueryForm from "@/features/AuthForm/AuthQueryForm";
import { Spinner } from "@/widgets";


const metadata: Metadata = {
    title: "Новый пароль",
};


const Page = () => {
    return <Suspense fallback={<Spinner label="Загружаем форму" />}><AuthQueryForm mode="reset-password" /></Suspense>;
};


export { metadata };
export default Page;
