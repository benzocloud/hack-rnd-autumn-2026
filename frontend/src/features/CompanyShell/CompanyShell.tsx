"use client";

import Link from "next/link";
import { useParams, usePathname } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, type ReactNode } from "react";
import { useDispatch } from "react-redux";

import { companyPath } from "@/helpers/finance";
import useResource from "@/hooks/useResource";
import type { IAccount, ICompany } from "@/interfaces";
import { companiesService, ledgerService } from "@/service";
import { setSelectedCompany } from "@/store/companySlice";
import { DataState, Icon, LinkButton } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


interface ICompanyContext {
    company: ICompany;
    account: IAccount;
    reload: () => void;
}


const CompanyContext = createContext<ICompanyContext | null>(null);
const navigation = [
    { page: "dashboard", title: "Обзор", icon: "layout-dashboard" },
    { page: "transactions", title: "Операции и данные", icon: "arrows-exchange" },
    { page: "calendar", title: "Календарь", icon: "calendar" },
    { page: "scenarios", title: "Сценарии", icon: "adjustments" },
    { page: "members", title: "Команда", icon: "users" },
    { page: "settings", title: "Компания", icon: "building" },
];


const useCompany = () => {
    const context = useContext(CompanyContext);

    if (!context) {
        throw new Error("Компания не выбрана.");
    }

    return context;
};


const CompanyShell = ( { children }: { children: ReactNode } ) => {
    const { companyId } = useParams<{ companyId: string }>();
    const pathname = usePathname();
    const dispatch = useDispatch();
    const loader = useCallback(async ( signal: AbortSignal ) => {
        const [ company, account ] = await Promise.all([
            companiesService.getCompany({ company_id: companyId }, signal),
            ledgerService.getDefaultAccount({ company_id: companyId }, signal),
        ]);

        return { company, account };
    }, [ companyId ]);
    const resource = useResource(loader);


    useEffect(() => {
        if (resource.data) {
            dispatch(setSelectedCompany(resource.data.company));

            try {
                localStorage.setItem("saldo-company-id", resource.data.company.id);
            } catch {}
        }
    }, [ resource.data, dispatch ]);


    return (
        <div className={styles.shell}>
            <div className={styles.companyLine}>
                <Link href="/workspace" className={styles.back}><Icon name="arrow-left" />Мои компании</Link>
                { resource.data && <span>{ resource.data.company.name } · { resource.data.company.membership.role === "owner" ? "Владелец" : "Финансист" }</span> }
            </div>
            <DataState loading={resource.loading && !resource.data} error={resource.error} onRetry={resource.reload}>
                { resource.data && (
                    <CompanyContext.Provider value={{ ...resource.data, reload: resource.reload }}>
                        <nav className={styles.navigation} aria-label="Разделы компании">
                            { navigation.map(( item ) => <Link key={item.page} href={companyPath(companyId, item.page)} aria-current={pathname.includes(`/${item.page}`) || (item.page === "scenarios" && pathname.includes("/forecasts/")) ? "page" : undefined}><Icon name={item.icon} />{ item.title }</Link>) }
                        </nav>
                        { children }
                    </CompanyContext.Provider>
                ) }
            </DataState>
            { resource.error && <div className={styles.recovery}><LinkButton href="/workspace" secondary>Выбрать другую компанию</LinkButton></div> }
        </div>
    );
};


export { useCompany };
export default CompanyShell;
