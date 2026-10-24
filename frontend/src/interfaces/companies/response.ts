import type { ICompany } from "@/interfaces";


interface ICompanyList {
    items: Array<ICompany>;
    next_cursor: string | null;
}


export type { ICompanyList };
