"use client";

import { useCallback, useState, type FormEvent } from "react";
import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { addDays, formatDate, formIdentity, invalid, sourceName, validDate } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { ICreateReconciliationGroupRequest, IReconciliationGroup } from "@/interfaces";
import { ledgerService, paymentsService } from "@/service";
import { ActionButton, DataState, Field, FormError, Modal, Notice, Panel, Select } from "@/widgets/FinanceUI/FinanceUI";
import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";
import styles from "./style.module.scss";

export const groupLabel = ( group: IReconciliationGroup ) => `${group.kind === "additional" ? "Дополнительное обязательство" : "Полная замена истории"} · ${formatDate(group.horizon_start)} — ${formatDate(group.horizon_end)} · ${group.id}`;

const ReconciliationControls = ( { groups, loading, error, onChanged }: { groups: IReconciliationGroup[]; loading: boolean; error: string | null; onChanged: () => void } ) => {
    const { company, account, reload } = useCompany();
    const [ creating, setCreating ] = useState(false);
    const [ revoking, setRevoking ] = useState<IReconciliationGroup | null>(null);
    const request = useRequest();
    const revoke = async () => {
        if (!revoking) { return; }
        const result = await request.run(( signal, key ) => paymentsService.revokeReconciliationGroup({ company_id: company.id, group_id: revoking.id, version: revoking.version, idempotency_key: key }, signal), "Согласование отозвано. Постройте новый прогноз.", `${revoking.id}:${revoking.version}`);
        if (result.ok) { setRevoking(null); onChanged(); reload(); }
    };
    return <Panel title="Учёт платежей в прогнозе" actions={<ActionButton variant="secondary" onClick={() => setCreating(true)}>Согласовать обязательства</ActionButton>}><Notice>Дополнительное обязательство прибавляется к историческому потоку. Полная замена исключает только явно выбранные исторические операции и требует подтверждения полного календаря на 30 дней. Категория и похожий текст не доказывают совпадение. После согласования выберите группу в форме платежа.</Notice><DataState loading={loading} error={error ? `Согласование недоступно: ${error} Без серверного подтверждения режим не применяется.` : null} onRetry={onChanged}>{ groups.map(( group ) => <div className={styles.item} key={group.id}><div><p>{ groupLabel(group) }</p><p>{ group.status === "active" ? "Действует" : "Отозвано" } · подтверждено { formatDate(group.confirmed_at) }</p>{ group.kind === "replacement" && <details><summary>Выбранные операции: { group.replacement.historical_operations.length }</summary>{ group.replacement.historical_operations.map(( item ) => <p key={item.transaction_id}>{ item.transaction_id } · версия { item.version }</p>) }</details> }</div>{ group.status === "active" && <ActionButton variant="secondary" onClick={() => { request.reset(); setRevoking(group); }}>Отозвать согласование</ActionButton> }</div>) }</DataState>{ creating && <CreateGroup initialCutoff={account.balance_as_of || ""} onClose={() => setCreating(false)} onCreated={() => { setCreating(false); onChanged(); reload(); }} /> }{ revoking && <Modal title="Отозвать согласование?" busy={request.busy} onClose={() => setRevoking(null)}><Notice tone="warning">В новом прогнозе выбранная история снова может учитываться. Отмена или полное исполнение платежа сами по себе не отзывают согласование. Сохранённые прогнозы не изменятся.</Notice><FormError message={request.error} /><ActionButton variant="danger" loading={request.busy} onClick={revoke}>Отозвать согласование</ActionButton></Modal> }</Panel>;
};

const CreateGroup = ( { initialCutoff, onClose, onCreated }: { initialCutoff: string; onClose: () => void; onCreated: () => void } ) => {
    const { company } = useCompany();
    const [ kind, setKind ] = useState("");
    const [ cutoff, setCutoff ] = useState(initialCutoff);
    const [ cursor, setCursor ] = useState<string | undefined>();
    const [ cursors, setCursors ] = useState<(string | undefined)[]>([]);
    const [ selected, setSelected ] = useState<Record<string, string>>({});
    const request = useRequest();
    const transactions = useResource(useCallback(( signal: AbortSignal ) => kind === "replacement" && validDate(cutoff) ? ledgerService.listTransactions({ company_id: company.id, from: addDays(cutoff, -89), to: cutoff, limit: 20, cursor }, signal) : Promise.resolve({ items: [], next_cursor: null }), [ company.id, kind, cutoff, cursor ]));
    const create = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const refs = Object.entries(selected).map(( [ transaction_id, version ] ) => ({ transaction_id, version }));
        const result = await request.run(( signal, key ) => {
            if (!validDate(cutoff)) { return invalid("Выберите дату среза.", "cutoff_date"); }
            if (!form.has("confirmed")) { return invalid("Подтвердите выбранный смысл согласования.", "confirmed"); }
            const window = { cutoff_date: cutoff, horizon_start: addDays(cutoff, 1), horizon_end: addDays(cutoff, 30) };
            let data: ICreateReconciliationGroupRequest;
            if (kind === "additional") { data = { ...window, kind, additional: { confirmed: true } }; }
            else if (kind === "replacement" && refs.length > 0 && refs.length <= 20000) { data = { ...window, kind, replacement: { complete_for_horizon: true, historical_operations: refs } }; }
            else { return invalid("Выберите режим и конкретные операции для полной замены.", "kind"); }
            return paymentsService.createReconciliationGroup({ company_id: company.id, idempotency_key: key, data }, signal);
        }, "Согласование сохранено", `${formIdentity(form)}:${JSON.stringify(refs)}`);
        if (result.ok) { onCreated(); }
    };
    return <Modal title="Согласовать обязательства" busy={request.busy} onClose={onClose}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={create}><fieldset disabled={request.busy}><Field name="cutoff_date" label="Дата среза" type="date" required value={cutoff} onChange={( event ) => { setCutoff(event.target.value); setSelected({}); setCursor(undefined); setCursors([]); }} /><Select name="kind" label="Как учитывать обязательства" value={kind} onChange={( event ) => setKind(event.target.value)}><option value="">Выберите явно</option><option value="additional">Дополнительное обязательство</option><option value="replacement">Полная замена выбранной истории</option></Select>{ validDate(cutoff) && <p>Окно календаря: { formatDate(addDays(cutoff, 1)) } — { formatDate(addDays(cutoff, 30)) }.</p> }{ kind === "replacement" && <DataState loading={transactions.loading} error={transactions.error} onRetry={transactions.reload}><p>Выберите операции по точным ID. Выбрано: { Object.keys(selected).length }. Выбор сохраняется между страницами.</p>{ transactions.data?.items.map(( item ) => <label className={styles.item} key={item.id}><input type="checkbox" checked={!!selected[item.id]} onChange={( event ) => setSelected(( current ) => event.target.checked ? { ...current, [item.id]: item.version } : Object.fromEntries(Object.entries(current).filter(( [ id ] ) => id !== item.id)))} /><span>{ formatDate(item.operation_date) } · { item.counterparty_name } · { item.direction === "inflow" ? "+" : "−" }{ formatMoney(item.amount.amount_minor) }<br />{ item.id } · версия { item.version } · { sourceName(item.source.kind) }</span></label>) }<div className={styles.row}><ActionButton variant="secondary" disabled={!cursors.length || transactions.loading} onClick={() => { setCursor(cursors.at(-1)); setCursors(( current ) => current.slice(0, -1)); }}>Назад</ActionButton><ActionButton variant="secondary" disabled={!transactions.data?.next_cursor || transactions.loading} onClick={() => { setCursors(( current ) => [ ...current, cursor ]); setCursor(transactions.data?.next_cursor || undefined); }}>Далее</ActionButton></div></DataState> }{ kind && <label key={`${kind}:${cutoff}:${Object.keys(selected).join(",")}`}><input type="checkbox" name="confirmed" />{ kind === "additional" ? "Подтверждаю, что это дополнительное обязательство сверх истории." : "Подтверждаю, что календарь полностью заменяет выбранные операции на всё указанное окно. Полнота не выведена автоматически." }</label> }</fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Сохранить согласование</ActionButton></ValidatedForm></Modal>;
};

export default ReconciliationControls;
