import type { ReactNode } from "react";

import CompanyShell from "@/features/CompanyShell/CompanyShell";


const CompanyLayout = ( { children }: { children: ReactNode } ) => <CompanyShell>{ children }</CompanyShell>;


export default CompanyLayout;
