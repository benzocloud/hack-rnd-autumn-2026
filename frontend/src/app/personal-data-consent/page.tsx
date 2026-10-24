import type { Metadata } from "next";

import { LegalDocument } from "@/features";


const metadata: Metadata = {
    title: "Согласие на обработку персональных данных",
};


const Page = () => {
    return <LegalDocument kind="consent" />;
};


export { metadata };
export default Page;
