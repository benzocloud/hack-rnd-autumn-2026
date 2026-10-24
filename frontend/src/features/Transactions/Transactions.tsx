"use client";

import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { companyPath, dateField, formatDate, formIdentity, invalid, parseMoney, sourceName, textField, today } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import { collectPages } from "@/helpers/pagination";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IListTransactionsParams, ITransaction } from "@/interfaces";
import { ledgerService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, LinkButton, Modal, Notice, PageHeading, Panel, Select, TextArea } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


const Transactions = () => {
    const { company, account, reload } = useCompany();
    const [ filters, setFilters ] = useState<Omit<IListTransactionsParams, "company_id">>({});
    const [ cursor, setCursor ] = useState<string | undefined>();
    const [ history, setHistory ] = useState<(string | undefined)[]>([]);
    const [ creating, setCreating ] = useState(false);
    const [ editing, setEditing ] = useState<ITransaction | null>(null);
    const request = useRequest();
    const categories = useResource(useCallback(( signal: AbortSignal ) => collectPages(( cursor ) => ledgerService.listCategories({ company_id: company.id, limit: 100, cursor }, signal)), [ company.id ]));
    const resource = useResource(useCallback(( signal: AbortSignal ) => ledgerService.listTransactions({ ...filters, company_id: company.id, limit: 20, cursor }, signal), [ company.id, filters, cursor ]));


    const filter = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);

        await request.run(async () => {
            const from = String(form.get("from") || "");
            const to = String(form.get("to") || "");

            if (from) { dateField(form, "from", "С даты"); }
            if (to) { dateField(form, "to", "По дату"); }
            if (from && to && from > to) { invalid("Дата начала не может быть позже окончания.", "from"); }

            setFilters({ from: from || undefined, to: to || undefined, direction: String(form.get("direction") || "") as "inflow" | "outflow" || undefined, category_id: String(form.get("category") || "") || undefined });
            setCursor(undefined);
            setHistory([]);
        });
    };


    const save = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(async ( signal, key ) => {
            const categoryId = String(form.get("category_id") || "") || null;

            if (editing) {
                return ledgerService.updateTransactionCategory({ company_id: company.id, transaction_id: editing.id, data: { category_id: categoryId, version: editing.version } }, signal);
            }

            const operationDate = dateField(form, "operation_date", "Дата операции");

            if (operationDate > today()) { invalid("Будущие операции добавляйте в платёжный календарь.", "operation_date"); }

            const latest = await ledgerService.getDefaultAccount({ company_id: company.id }, signal);

            return ledgerService.createManualTransaction({ company_id: company.id, idempotency_key: key, data: { operation_date: operationDate, direction: form.get("direction") === "inflow" ? "inflow" : "outflow", amount: { amount_minor: parseMoney(textField(form, "amount", "Сумма")), currency: "RUB" }, counterparty_name: textField(form, "counterparty_name", "Контрагент"), payment_purpose: textField(form, "payment_purpose", "Назначение платежа", 1000), category_id: categoryId, expected_data_version: latest.data_version } }, signal);
        }, editing ? "Категория сохранена" : "Операция добавлена", formIdentity(form));

        if (result.ok) {
            setCreating(false);
            setEditing(null);
            resource.reload();
            reload();
        }
    };


    return <main className={styles.page}><PageHeading title="Операции и данные" description="Фактические поступления и списания. Будущие платежи находятся в календаре."><LinkButton href={companyPath(company.id, "imports")} icon="file-upload" secondary>Импортировать</LinkButton><ActionButton icon="plus" onClick={() => setCreating(true)} disabled={account.transaction_source?.mode === "external"}>Добавить операцию</ActionButton></PageHeading><div className={styles.stack}>{ account.transaction_source?.mode === "external" && <Notice>Операции поступают из внешнего источника. Для сохранения целостности данных ручное добавление недоступно; категории можно редактировать.</Notice> }<Panel><form noValidate onSubmit={filter} className={styles.filters}><Field name="from" label="С даты" type="date" /><Field name="to" label="По дату" type="date" /><Select disabled={request.busy} name="direction" label="Движение"><option value="">Все операции</option><option value="inflow">Поступления</option><option value="outflow">Списания</option></Select><Select disabled={request.busy} name="category" label="Категория"><option value="">Все категории</option>{ categories.data?.map(( item ) => <option key={item.id} value={item.id}>{ item.name }</option>) }</Select><ActionButton type="submit" variant="secondary" loading={request.busy}>Применить</ActionButton></form></Panel><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}><Panel>{ resource.data?.items.length ? <><div className={styles.scroll}><table className={styles.table}><thead><tr><th>Дата</th><th>Контрагент / назначение</th><th>Сумма</th><th>Категория</th><th>Источник</th><th><span className={styles.muted}>Действие</span></th></tr></thead><tbody>{ resource.data.items.map(( item ) => <tr key={item.id}><td>{ formatDate(item.operation_date) }</td><td><strong>{ item.counterparty_name }</strong><div className={styles.muted}>{ item.payment_purpose }</div></td><td className={`${styles.money} ${styles[item.direction]}`}>{ item.direction === "inflow" ? "+" : "−" }{ formatMoney(item.amount.amount_minor) }</td><td>{ categories.data?.find(( category ) => category.id === item.category_id)?.name || "Без категории" }</td><td><Badge>{ sourceName(item.source.kind) }</Badge></td><td><ActionButton variant="secondary" icon="pencil" onClick={() => setEditing(item)} aria-label={`Изменить категорию операции «${item.counterparty_name}»`}>Категория</ActionButton></td></tr>) }</tbody></table></div><div className={styles.actions}>{ history.length > 0 && <ActionButton variant="secondary" onClick={() => { setCursor(history[history.length - 1]); setHistory(history.slice(0, -1)); }}>Назад</ActionButton> }{ resource.data.next_cursor && <ActionButton variant="secondary" onClick={() => { setHistory([ ...history, cursor ]); setCursor(resource.data?.next_cursor || undefined); }}>Следующие операции</ActionButton> }</div></> : <EmptyState title="Операций пока нет" icon="arrows-exchange">Загрузите выписку или добавьте операцию вручную. Если вы применили фильтры, попробуйте изменить период.</EmptyState> }</Panel></DataState></div>{ (creating || editing) && <Modal title={editing ? "Категория операции" : "Новая операция"} busy={request.busy} onClose={() => { setCreating(false); setEditing(null); }}><form className={styles.form} noValidate onSubmit={save}><fieldset disabled={request.busy}>{ !editing && <><Select disabled={request.busy} name="direction" label="Движение"><option value="outflow">Списание</option><option value="inflow">Поступление</option></Select><Field name="amount" label="Сумма, ₽" inputMode="decimal" placeholder="0,00" /><Field name="operation_date" label="Дата операции" type="date" defaultValue={today()} /><Field name="counterparty_name" label="Контрагент" placeholder="Название или имя" /><TextArea name="payment_purpose" label="Назначение платежа" /></> }<Select disabled={request.busy} name="category_id" label="Категория" defaultValue={editing?.category_id || ""}><option value="">Без категории</option>{ categories.data?.filter(( category ) => !editing || !category.direction || category.direction === editing.direction).map(( item ) => <option key={item.id} value={item.id}>{ item.name }</option>) }</Select></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>{ editing ? "Сохранить категорию" : "Добавить операцию" }</ActionButton></form></Modal> }</main>;
};


export default Transactions;
