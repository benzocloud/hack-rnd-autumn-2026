import type { Metadata } from "next";

import { LegalDocument } from "@/features";


const metadata: Metadata = {
    title: "Политика конфиденциальности",
};


const Page = () => {
    return <LegalDocument kind="privacy" />;
};


export { metadata };
export default Page;
