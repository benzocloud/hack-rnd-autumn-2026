"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useState, type FormEvent } from "react";

import { companyPath, formatDate, formIdentity, invalid, textField } from "@/helpers/finance";
import { collectPages } from "@/helpers/pagination";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IInvitation } from "@/interfaces";
import { companiesService, membersService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, Icon, Modal, PageHeading, Panel } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


const Workspace = () => {
    const router = useRouter();
    const [ creating, setCreating ] = useState(false);
    const [ declined, setDeclined ] = useState<IInvitation | null>(null);
    const request = useRequest();
    const loader = useCallback(async ( signal: AbortSignal ) => {
        const [ companies, invitations ] = await Promise.all([
            collectPages(( cursor ) => companiesService.listCompanies({ limit: 100, cursor }, signal)),
            collectPages(( cursor ) => membersService.listMyInvitations({ limit: 100, cursor }, signal)),
        ]);

        return { companies, invitations };
    }, []);
    const resource = useResource(loader);


    const createCompany = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const name = textField(form, "name", "Название компании");
            const inn = textField(form, "inn", "ИНН", 12, true);

            if (inn && !/^([0-9]{10}|[0-9]{12})$/.test(inn)) {
                invalid("ИНН должен содержать 10 или 12 цифр.", "inn");
            }

            return companiesService.createCompany({ idempotency_key: key, data: { name, inn: inn || null, currency: "RUB", timezone: "Europe/Moscow" } }, signal);
        }, "Компания создана", formIdentity(form));

        if (result.ok) {
            router.push(companyPath(result.data.id, "settings"));
        }
    };


    const accept = async ( invitation: IInvitation ) => {
        const result = await request.run(( signal, key ) => membersService.acceptInvitation({ invitation_id: invitation.id, idempotency_key: key, data: { version: invitation.version } }, signal), "Приглашение принято", invitation.id);

        if (result.ok) {
            router.push(companyPath(result.data.id));
        }
    };


    const decline = async () => {
        if (!declined) {
            return;
        }

        const result = await request.run(( signal ) => membersService.declineInvitation({ invitation_id: declined.id, data: { version: declined.version } }, signal), "Приглашение отклонено", declined.id);

        if (result.ok) {
            setDeclined(null);
            resource.reload();
        }
    };


    return (
        <main className={styles.page}>
            <PageHeading title="Мои компании" description="Выберите компанию, чтобы работать с её финансами. Данные разных компаний хранятся отдельно.">
                <ActionButton icon="plus" onClick={() => setCreating(true)}>Создать компанию</ActionButton>
            </PageHeading>
            <DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>
                { resource.data && (
                    <>
                        { resource.data.invitations.some(( item ) => item.status === "pending") && <Panel title="Вас приглашают в команду"><div className={styles.stack}>{ resource.data.invitations.filter(( item ) => item.status === "pending").map(( item ) => <div key={item.id} className={styles.invitation}><div><h3>{ item.company_name }</h3><p>Финансист · приглашение до { formatDate(item.expires_at) }</p></div><div className={styles.actions}><ActionButton loading={request.busy} onClick={() => accept(item)}>Принять</ActionButton><ActionButton variant="secondary" disabled={request.busy} onClick={() => setDeclined(item)}>Отклонить</ActionButton></div></div>) }</div></Panel> }
                        { resource.data.companies.length ? <div className={styles.grid}>{ resource.data.companies.map(( company ) => <Link key={company.id} className={styles.company} href={companyPath(company.id)}><div className={styles.companyTop}><span className={styles.avatar}>{ Array.from(company.name)[0] }</span><Icon name="arrow-right" /></div><h2>{ company.name }</h2><p>{ company.inn ? `ИНН ${company.inn}` : "ИНН не указан" }</p><Badge>{ company.membership.role === "owner" ? "Владелец" : "Финансист" }</Badge></Link>) }</div> : <Panel><EmptyState title="Начнём с вашей компании" icon="building" action={<ActionButton onClick={() => setCreating(true)}>Создать первую компанию</ActionButton>}>После создания укажите остаток на счёте, загрузите операции и добавьте будущие платежи.</EmptyState></Panel> }
                    </>
                ) }
            </DataState>
            { creating && <Modal title="Новая компания" busy={request.busy} onClose={() => setCreating(false)}><form className={styles.form} noValidate onSubmit={createCompany}><Field name="name" label="Название компании" placeholder="Например, ООО «Вектор»" autoComplete="organization" autoFocus disabled={request.busy} /><Field name="inn" label="ИНН — необязательно" inputMode="numeric" placeholder="10 или 12 цифр" disabled={request.busy} /><p>Валюта: российский рубль. Часовой пояс: Москва.</p><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Создать компанию</ActionButton></form></Modal> }
            { declined && <Modal title="Отклонить приглашение?" busy={request.busy} onClose={() => setDeclined(null)}><p>Вы не присоединитесь к компании «{ declined.company_name }». Владелец сможет пригласить вас повторно.</p><div className={styles.actions}><ActionButton variant="secondary" disabled={request.busy} onClick={() => setDeclined(null)}>Оставить</ActionButton><ActionButton variant="danger" loading={request.busy} onClick={decline}>Отклонить</ActionButton></div><FormError message={request.error} /></Modal> }
        </main>
    );
};


export default Workspace;
