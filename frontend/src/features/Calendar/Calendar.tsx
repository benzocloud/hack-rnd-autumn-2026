"use client";

import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { addDays, dateField, formatDate, formIdentity, invalid, moneyInput, parseMoney, sourceName, textField, today } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import { collectPages } from "@/helpers/pagination";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IPayment } from "@/interfaces";
import { ledgerService, paymentsService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, Modal, Notice, PageHeading, Panel, Select, TextArea } from "@/widgets/FinanceUI/FinanceUI";

import { CreateForecastButton } from "@/features/ForecastDetail/ForecastDetail";
import ReconciliationControls, { groupLabel } from "./ReconciliationControls";

import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


const statuses = { planned: "Запланирован", partially_matched: "Частично исполнен", matched: "Исполнен", cancelled: "Отменён" };


const Calendar = () => {
    const { company, reload } = useCompany();
    const [ paymentStatus, setPaymentStatus ] = useState<IPayment["status"] | "">("");
    const [ month, setMonth ] = useState(() => today().slice(0, 7));
    const [ selectedDay, setSelectedDay ] = useState<string | null>(null);
    const [ view, setView ] = useState<"calendar" | "list">("calendar");
    const [ editing, setEditing ] = useState<IPayment | "new" | null>(null);
    const [ cancelled, setCancelled ] = useState<IPayment | null>(null);
    const [ matched, setMatched ] = useState<IPayment | null>(null);
    const request = useRequest();
    const start = `${month}-01`;
    const end = new Date(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5)), 0)).toISOString().slice(0, 10);
    const resource = useResource(useCallback(( signal: AbortSignal ) => collectPages(( cursor ) => paymentsService.listPayments({ company_id: company.id, from: start, to: end, status: paymentStatus || undefined, limit: 100, cursor }, signal)), [ company.id, start, end, paymentStatus ]));
    const categories = useResource(useCallback(( signal: AbortSignal ) => collectPages(( cursor ) => ledgerService.listCategories({ company_id: company.id, limit: 100, cursor }, signal)), [ company.id ]));
    const groups = useResource(useCallback(( signal: AbortSignal ) => collectPages(( cursor ) => paymentsService.listReconciliationGroups({ company_id: company.id, include_revoked: true, cursor }, signal)), [ company.id ]));
    const visible = (resource.data || []).filter(( item ) => !selectedDay || item.scheduled_date === selectedDay);
    const offset = (new Date(`${start}T00:00:00Z`).getUTCDay() + 6) % 7;
    const days = Array.from({ length: Number(end.slice(8)) }, ( _, index ) => addDays(start, index));
    const currentPayment = editing && editing !== "new" ? editing : null;


    const changeMonth = ( increment: number ) => {
        const date = new Date(`${start}T00:00:00Z`);

        date.setUTCMonth(date.getUTCMonth() + increment);
        setMonth(date.toISOString().slice(0, 7));
        setSelectedDay(null);
    };


    const save = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const groupId = String(form.get("reconciliation_group_id") || "");
            if (!groupId) { return invalid("Явно выберите согласование или несогласованный режим.", "reconciliation_group_id"); }
            const data = { reconciliation_group_id: groupId === "unresolved" ? null : groupId, direction: currentPayment?.status === "partially_matched" ? currentPayment.direction : form.get("direction") === "inflow" ? "inflow" as const : "outflow" as const, amount: currentPayment?.status === "partially_matched" ? currentPayment.amount : { amount_minor: parseMoney(textField(form, "amount", "Сумма")), currency: "RUB" as const }, scheduled_date: dateField(form, "scheduled_date", "Дата платежа"), counterparty_name: textField(form, "counterparty_name", "Контрагент"), description: textField(form, "description", "Описание", 1000), category_id: String(form.get("category_id") || "") || null };

            return currentPayment ? paymentsService.updatePayment({ company_id: company.id, payment_id: currentPayment.id, data: currentPayment.status === "partially_matched" ? { scheduled_date: data.scheduled_date, counterparty_name: data.counterparty_name, description: data.description, category_id: data.category_id, reconciliation_group_id: data.reconciliation_group_id, version: currentPayment.version } : { ...data, version: currentPayment.version } }, signal) : paymentsService.createPayment({ company_id: company.id, idempotency_key: key, data }, signal);
        }, currentPayment ? "Платёж изменён" : "Платёж добавлен", formIdentity(form));

        if (result.ok) {
            setEditing(null);
            setMonth(result.data.scheduled_date.slice(0, 7));
            setSelectedDay(result.data.scheduled_date);
            resource.reload();
            reload();
        }
    };


    const cancel = async () => {
        if (!cancelled) { return; }

        const result = await request.run(( signal ) => paymentsService.cancelPayment({ company_id: company.id, payment_id: cancelled.id, version: cancelled.version }, signal), "Платёж отменён", cancelled.id);

        if (result.ok) { setCancelled(null); resource.reload(); reload(); }
    };


    return <main className={styles.page}><PageHeading title="Платёжный календарь" description="Планируйте поступления и списания. Исполнение связывайте с фактическими операциями."><ActionButton icon="plus" onClick={() => setEditing("new")}>Добавить платёж</ActionButton></PageHeading><div className={styles.stack}><Notice>Изменение календаря делает сохранённый прогноз неактуальным. Новый расчёт создаёт отдельный результат; старый снимок не меняется.</Notice><CreateForecastButton /><ReconciliationControls groups={groups.data || []} loading={groups.loading} error={groups.error} onChanged={groups.reload} /><Panel><div className={styles.calendarToolbar}><div className={styles.row}><ActionButton variant="secondary" icon="arrow-left" aria-label="Предыдущий месяц" onClick={() => changeMonth(-1)} /><h2>{ new Intl.DateTimeFormat("ru-RU", { month: "long", year: "numeric", timeZone: "UTC" }).format(new Date(`${start}T00:00:00Z`)) }</h2><ActionButton variant="secondary" icon="arrow-right" aria-label="Следующий месяц" onClick={() => changeMonth(1)} /></div><div className={styles.row}><ActionButton variant="secondary" aria-pressed={view === "calendar"} onClick={() => setView("calendar")}>Месяц</ActionButton><ActionButton variant="secondary" aria-pressed={view === "list"} onClick={() => { setView("list"); setSelectedDay(null); }}>Список</ActionButton></div></div><Select name="payment_status" label="Состояние платежей" value={paymentStatus} onChange={( event ) => setPaymentStatus(event.target.value as IPayment["status"] | "")}><option value="">Активные</option><option value="planned">Запланированные</option><option value="partially_matched">Частично исполненные</option><option value="matched">Исполненные</option><option value="cancelled">Отменённые</option></Select><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ view === "calendar" && <div className={styles.calendarGrid} role="group" aria-label="Выбор дня">{ [ "Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс" ].map(( day ) => <span key={day} className={styles.weekday}>{ day }</span>) }{ Array.from({ length: offset }, ( _, index ) => <span key={`empty-${index}`} />) }{ days.map(( day ) => { const payments = resource.data?.filter(( item ) => item.scheduled_date === day && item.status !== "cancelled") || []; return <button className={styles.day} key={day} type="button" aria-pressed={selectedDay === day} aria-current={day === today() ? "date" : undefined} aria-label={`${formatDate(day)}, платежей: ${payments.length}`} onClick={() => setSelectedDay(selectedDay === day ? null : day)}><span>{ Number(day.slice(8)) }</span><div className={styles.dayDots}>{ payments.some(( item ) => item.direction === "inflow") && <i className={styles.inflowDot} /> }{ payments.some(( item ) => item.direction === "outflow") && <i className={styles.outflowDot} /> }</div>{ payments.length > 0 && <small>{ payments.length } плат.</small> }</button>; }) }</div> }</DataState></Panel><Panel title={selectedDay ? `Платежи на ${formatDate(selectedDay)}` : "Платежи за месяц"} actions={selectedDay && <ActionButton variant="secondary" onClick={() => setSelectedDay(null)}>Все дни</ActionButton>}><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ visible.length ? <div>{ visible.map(( item ) => <div className={styles.item} key={item.id}><div><h3>{ item.counterparty_name }</h3><p className={styles.muted}>{ formatDate(item.scheduled_date) } · { item.description }</p><Badge tone={item.status === "matched" ? "success" : "neutral"}>{ statuses[item.status] }</Badge></div><div><p className={`${styles.money} ${styles[item.direction]}`}>{ item.direction === "inflow" ? "+" : "−" }{ formatMoney(item.amount.amount_minor) }</p><p className={styles.muted}>Осталось { formatMoney(item.remaining_amount.amount_minor) } · { sourceName(item.source.kind) }</p><p className={styles.muted}>Согласование: { item.reconciliation_group_id || "не задано; участие в прогнозе не подтверждено" }</p><div className={styles.row}><ActionButton variant="secondary" onClick={() => setMatched(item)}>{ item.status === "cancelled" ? "История исполнения" : "Исполнение" }</ActionButton>{ [ "planned", "partially_matched" ].includes(item.status) && <><ActionButton variant="secondary" icon="pencil" onClick={() => setEditing(item)}>Изменить</ActionButton><ActionButton variant="secondary" onClick={() => setCancelled(item)}>Отменить</ActionButton></> }</div></div></div>) }</div> : <EmptyState title={selectedDay ? "На этот день платежей нет" : "Месяц пока свободен"} icon="calendar" action={<ActionButton onClick={() => setEditing("new")}>Запланировать платёж</ActionButton>}>Добавьте ожидаемое поступление или будущий расход и явно согласуйте его учёт в новом прогнозе.</EmptyState> }</DataState></Panel></div>{ editing && <Modal title={currentPayment ? "Изменить платёж" : "Новый платёж"} busy={request.busy} onClose={() => setEditing(null)}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={save}><Notice>Сначала согласуйте additional/replacement в разделе «Учёт платежей в прогнозе». Без группы сервер может отклонить финансовый прогноз. Изменение суммы и направления после частичного исполнения запрещено.</Notice><fieldset disabled={request.busy}><Select name="reconciliation_group_id" label="Согласование с историей" defaultValue={currentPayment ? currentPayment.reconciliation_group_id || "unresolved" : ""}><option value="">Выберите явно</option><option value="unresolved">Не согласовано — учёт в прогнозе не подтверждён</option>{ currentPayment?.reconciliation_group_id && !groups.data?.some(( group ) => group.id === currentPayment.reconciliation_group_id) && <option value={currentPayment.reconciliation_group_id}>{ currentPayment.reconciliation_group_id } · текущая группа</option> }{ groups.data?.filter(( group ) => group.status === "active" || group.id === currentPayment?.reconciliation_group_id).map(( group ) => <option value={group.id} key={group.id}>{ groupLabel(group) }{ group.status === "revoked" ? " · отозвано" : "" }</option>) }</Select><Select disabled={request.busy || currentPayment?.status === "partially_matched"} name="direction" label="Движение" defaultValue={currentPayment?.direction || "outflow"}><option value="outflow">Списание</option><option value="inflow">Поступление</option></Select><Field required data-maximum="200" disabled={currentPayment?.status === "partially_matched"} name="amount" label="Сумма, ₽" inputMode="decimal" placeholder="0,00" defaultValue={currentPayment ? moneyInput(currentPayment.amount.amount_minor) : ""} /><Field required data-maximum="200" name="scheduled_date" label="Дата платежа" type="date" defaultValue={currentPayment?.scheduled_date || selectedDay || today()} /><Field required data-maximum="200" name="counterparty_name" label="Контрагент" defaultValue={currentPayment?.counterparty_name} /><TextArea required data-maximum="1000" name="description" label="Описание платежа" defaultValue={currentPayment?.description} /><Select disabled={request.busy} name="category_id" label="Категория" defaultValue={currentPayment?.category_id || ""}><option value="">Без категории</option>{ categories.data?.map(( item ) => <option key={item.id} value={item.id}>{ item.name }</option>) }</Select></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Сохранить платёж</ActionButton></ValidatedForm></Modal> }{ cancelled && <Modal title="Отменить платёж?" busy={request.busy} onClose={() => setCancelled(null)}><p>«{ cancelled.counterparty_name }» — неисполненная часть { formatMoney(cancelled.remaining_amount.amount_minor) } будет отменена. Фактические операции сохранятся.</p><div className={styles.actions}><ActionButton variant="secondary" disabled={request.busy} onClick={() => setCancelled(null)}>Оставить платёж</ActionButton><ActionButton variant="danger" loading={request.busy} onClick={cancel}>Отменить платёж</ActionButton></div><FormError message={request.error} /></Modal> }{ matched && <PaymentExecution payment={matched} onClose={() => setMatched(null)} onChanged={() => { resource.reload(); reload(); }} /> }</main>;
};


const PaymentExecution = ( { payment, onClose, onChanged }: { payment: IPayment; onClose: () => void; onChanged: () => void } ) => {
    const { company } = useCompany();
    const request = useRequest();
    const resource = useResource(useCallback(async ( signal: AbortSignal ) => {
        const [ current, matches, transactions ] = await Promise.all([
            paymentsService.getPayment({ company_id: company.id, payment_id: payment.id }, signal),
            collectPages(( cursor ) => paymentsService.listPaymentMatches({ company_id: company.id, payment_id: payment.id, limit: 100, cursor }, signal)),
            collectPages(( cursor ) => ledgerService.listTransactions({ company_id: company.id, direction: payment.direction, limit: 100, cursor }, signal)),
        ]);

        return { current, matches, transactions };
    }, [ company.id, payment.id, payment.direction ]));


    const match = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const transaction = resource.data?.transactions.find(( item ) => item.id === form.get("transaction"));
            const amount = parseMoney(textField(form, "amount", "Сумма"));

            if (!transaction || !resource.data) { return invalid("Выберите фактическую операцию.", "transaction"); }
            if (BigInt(amount) > BigInt(resource.data.current.remaining_amount.amount_minor) || BigInt(amount) > BigInt(transaction.amount.amount_minor)) { return invalid("Сумма превышает остаток платежа или сумму операции.", "amount"); }

            return paymentsService.matchPayment({ company_id: company.id, payment_id: payment.id, idempotency_key: key, data: { transaction_id: transaction.id, amount: { amount_minor: amount, currency: "RUB" }, payment_version: resource.data.current.version, transaction_version: transaction.version } }, signal);
        }, "Исполнение связано с операцией", formIdentity(form));

        if (result.ok) { resource.reload(); onChanged(); }
    };


    const unmatch = async ( id: string ) => {
        if (!resource.data) { return; }
        const version = resource.data.current.version;
        const result = await request.run(( signal ) => paymentsService.removePaymentMatch({ company_id: company.id, payment_id: payment.id, match_id: id, payment_version: version }, signal), "Связь удалена, фактическая операция сохранена", id);

        if (result.ok) { resource.reload(); onChanged(); }
    };


    return <Modal title="Исполнение платежа" busy={request.busy} onClose={onClose}><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data && <div className={styles.stack}><Notice>Связь отмечает исполнение фактом и не проводит платёж. Осталось исполнить: { formatMoney(resource.data.current.remaining_amount.amount_minor) }.</Notice>{ resource.data.matches.map(( item ) => <div className={styles.item} key={item.id}><span>{ formatMoney(item.amount.amount_minor) } · операция { item.transaction_id } · { formatDate(item.created_at) }{ item.removed_at ? ` · связь снята ${formatDate(item.removed_at)}` : " · действующая связь" }</span>{ resource.data?.current.status !== "cancelled" && !item.removed_at && <ActionButton variant="secondary" loading={request.busy} onClick={() => unmatch(item.id)}>Убрать связь</ActionButton> }</div>) }{ resource.data.current.status === "cancelled" && <Notice>Платёж отменён. История связей сохранена; новые связи и unmatch запрещены.</Notice> }{ [ "planned", "partially_matched" ].includes(resource.data.current.status) && BigInt(resource.data.current.remaining_amount.amount_minor) > BigInt(0) && <ValidatedForm request={request} noValidate className={styles.form} onSubmit={match}><fieldset disabled={request.busy}><Select disabled={request.busy} name="transaction" label="Фактическая операция"><option value="">Выберите операцию</option>{ resource.data.transactions.map(( item ) => <option value={item.id} key={item.id}>{ formatDate(item.operation_date) } · { item.counterparty_name } · { formatMoney(item.amount.amount_minor) }</option>) }</Select><Field required data-maximum="200" name="amount" label="Исполненная сумма, ₽" inputMode="decimal" placeholder="0,00" defaultValue={moneyInput(resource.data.current.remaining_amount.amount_minor)} /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy} disabled={!resource.data.transactions.length}>Связать с операцией</ActionButton>{ !resource.data.transactions.length && <p className={styles.muted}>Сначала загрузите или добавьте фактическую операцию того же направления.</p> }</ValidatedForm> }</div> }</DataState></Modal>;
};


export default Calendar;
