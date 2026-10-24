"use client";

import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { formatDate, formIdentity, invalid, textField } from "@/helpers/finance";
import { collectPages } from "@/helpers/pagination";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import { membersService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, Modal, Notice, PageHeading, Panel } from "@/widgets/FinanceUI/FinanceUI";

import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


const Members = () => {
    const { company } = useCompany();
    const [ inviting, setInviting ] = useState(false);
    const [ removal, setRemoval ] = useState<{ id: string; name: string; invitation: boolean } | null>(null);
    const request = useRequest();
    const owner = company.membership.role === "owner";
    const loader = useCallback(async ( signal: AbortSignal ) => {
        const members = await collectPages(( cursor ) => membersService.listMembers({ company_id: company.id, limit: 100, cursor }, signal));
        const invitations = owner ? await collectPages(( cursor ) => membersService.listCompanyInvitations({ company_id: company.id, limit: 100, cursor }, signal)) : [];

        return { members, invitations };
    }, [ company.id, owner ]);
    const resource = useResource(loader);


    const invite = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const email = textField(form, "email", "Почта", 254).toLowerCase();

            if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
                invalid("Укажите корректный адрес почты.", "email");
            }

            return membersService.createInvitation({ company_id: company.id, idempotency_key: key, data: { email } }, signal);
        }, "Приглашение отправлено", formIdentity(form));

        if (result.ok) {
            setInviting(false);
            resource.reload();
        }
    };


    const remove = async () => {
        if (!removal) {
            return;
        }

        const result = await request.run(( signal ) => removal.invitation ? membersService.revokeInvitation({ company_id: company.id, invitation_id: removal.id }, signal) : membersService.removeMember({ company_id: company.id, user_id: removal.id }, signal), removal.invitation ? "Приглашение отозвано" : "Доступ участника закрыт", removal.id);

        if (result.ok) {
            setRemoval(null);
            resource.reload();
        }
    };


    return <main className={styles.page}><PageHeading title="Команда" description="Совместная работа с финансами компании.">{ owner && <ActionButton icon="plus" onClick={() => setInviting(true)}>Пригласить финансиста</ActionButton> }</PageHeading>{ !owner && <Notice>У вас роль финансиста. Вы можете работать с данными и прогнозами; управлять доступом может владелец компании.</Notice> }<DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && <div className={styles.stack}><Panel title="Участники">{ resource.data.members.map(( member ) => <div className={styles.item} key={member.user_id}><div><h3>{ member.display_name }</h3><p>{ member.email }</p></div><div className={styles.row}><Badge>{ member.role === "owner" ? "Владелец" : "Финансист" }</Badge>{ owner && member.role !== "owner" && <ActionButton variant="secondary" icon="trash" onClick={() => setRemoval({ id: member.user_id, name: member.display_name, invitation: false })}>Закрыть доступ</ActionButton> }</div></div>) }</Panel>{ owner && <Panel title="Ожидают приглашения">{ resource.data.invitations.some(( item ) => item.status === "pending") ? resource.data.invitations.filter(( item ) => item.status === "pending").map(( item ) => <div className={styles.item} key={item.id}><div><h3>{ item.email }</h3><p className={styles.muted}>Финансист · до { formatDate(item.expires_at) }</p></div><ActionButton variant="secondary" onClick={() => setRemoval({ id: item.id, name: item.email, invitation: true })}>Отозвать</ActionButton></div>) : <EmptyState title="Ожидающих приглашений нет" icon="users">Пригласите коллегу, у которого уже есть учётная запись в Saldo.</EmptyState> }</Panel> }</div> }</DataState>{ inviting && <Modal title="Пригласить финансиста" busy={request.busy} onClose={() => setInviting(false)}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={invite}><p>Коллега должен быть зарегистрирован в Saldo. Он получит доступ после принятия приглашения в разделе «Мои компании».</p><Field required data-maximum="254" name="email" label="Почта коллеги" type="email" placeholder="name@company.ru" disabled={request.busy} autoFocus /><Notice>Финансист сможет видеть операции, импортировать данные, создавать платежи и прогнозы. Управление командой останется у владельца.</Notice><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Отправить приглашение</ActionButton></ValidatedForm></Modal> }{ removal && <Modal title={removal.invitation ? "Отозвать приглашение?" : "Закрыть доступ?"} busy={request.busy} onClose={() => setRemoval(null)}><p>{ removal.name } { removal.invitation ? "не сможет принять это приглашение." : "потеряет доступ к данным компании. Созданные им данные сохранятся." }</p><div className={styles.actions}><ActionButton variant="secondary" disabled={request.busy} onClick={() => setRemoval(null)}>Отмена</ActionButton><ActionButton variant="danger" loading={request.busy} onClick={remove}>Подтвердить</ActionButton></div><FormError message={request.error} /></Modal> }</main>;
};


export default Members;
