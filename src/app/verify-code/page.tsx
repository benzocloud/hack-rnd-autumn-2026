import type { Metadata } from "next";
import { Suspense } from "react";

import AuthQueryForm from "@/features/AuthForm/AuthQueryForm";
import { Spinner } from "@/widgets";


const metadata: Metadata = {
    title: "Подтверждение кодом",
};


const Page = () => {
    return <Suspense fallback={<Spinner label="Загружаем форму" />}><AuthQueryForm mode="verify-code" /></Suspense>;
};


export { metadata };
export default Page;
