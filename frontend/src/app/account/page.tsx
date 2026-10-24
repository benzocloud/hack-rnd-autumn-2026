import type { Metadata } from "next";

import { Account } from "@/features";


const metadata: Metadata = {
    title: "Личный кабинет",
};


const Page = () => {
    return <Account />;
};


export { metadata };
export default Page;
