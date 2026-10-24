import type { Metadata } from "next";

import Imports from "@/features/Imports/Imports";


const metadata: Metadata = { title: "Импорт данных" };


const ImportsPage = async ( { searchParams }: { searchParams: Promise<{ import?: string }> } ) => {
    const params = await searchParams;

    return <Imports key={params.import || "new"} importId={params.import} />;
};


export { metadata };
export default ImportsPage;
