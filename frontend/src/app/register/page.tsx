import type { Metadata } from "next";

import consentDocument from "@/content/personal-data-consent.json";
import { AuthForm } from "@/features";


const metadata: Metadata = {
    title: "Регистрация",
};


const Page = () => {
    return <AuthForm mode="register" consentDocument={{ document_version: consentDocument.document_version, document_sha256: consentDocument.document_sha256 }} />;
};


export { metadata };
export default Page;
