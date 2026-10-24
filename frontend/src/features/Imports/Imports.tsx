"use client";

import { useRouter } from "next/navigation";
import { useCallback, useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { addDays, companyPath, dateField, formatDate, formIdentity, invalid, textField, today } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import { collectPages } from "@/helpers/pagination";
import useJobResource from "@/hooks/useJobResource";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IConnection, IImportRecordKinds } from "@/interfaces";
import { importsService, integrationsService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, JobState, Modal, Notice, PageHeading, Panel, Select } from "@/widgets/FinanceUI/FinanceUI";

import styles from "./style.module.scss";


const importStatus = { queued: "В очереди", processing: "Проверяется", preview_ready: "Ожидает подтверждения", applied: "Данные добавлены", failed: "Ошибка" };


const Imports = ( { importId = "" }: { importId?: string } ) => {
    const { company, account } = useCompany();
    const router = useRouter();
    const [ tab, setTab ] = useState<"file" | "external">("file");
    const [ adding, setAdding ] = useState(false);
    const [ disabled, setDisabled ] = useState<IConnection | null>(null);
    const [ selectedConnection, setSelectedConnection ] = useState("");
    const request = useRequest();
    const resource = useResource(useCallback(async ( signal: AbortSignal ) => {
        const [ history, providers, connections ] = await Promise.all([
            collectPages(( cursor ) => importsService.listImports({ company_id: company.id, limit: 100, cursor }, signal)),
            collectPages(( cursor ) => integrationsService.listExternalProviders({ company_id: company.id, limit: 100, cursor }, signal)),
            collectPages(( cursor ) => integrationsService.listConnections({ company_id: company.id, limit: 100, cursor }, signal)),
        ]);

        return { history, providers, connections };
    }, [ company.id ]));
    const connection = resource.data?.connections.find(( item ) => item.id === selectedConnection);
    const provider = resource.data?.providers.find(( item ) => item.id === connection?.provider_id);


    const openImport = ( id: string ) => router.push(`${companyPath(company.id, "imports")}?import=${encodeURIComponent(id)}`, { scroll: false });


    const upload = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const file = form.get("file");

            if (!(file instanceof File) || !file.size) { return invalid("Выберите непустой CSV-файл.", "file"); }
            if (!file.name.toLowerCase().endsWith(".csv")) { return invalid("Поддерживаются выписки в формате CSV (dataset_v1).", "file"); }

            return importsService.uploadImport({ company_id: company.id, idempotency_key: key, data: { file, adapter: "dataset_v1", source_company_id: textField(form, "source_company_id", "ID компании в файле"), account_id: account.id, currency: "RUB" } }, signal);
        }, "Файл принят на проверку", formIdentity(form));

        if (result.ok) { openImport(result.data.id); }
    };


    const createConnection = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => integrationsService.createConnection({ company_id: company.id, idempotency_key: key, data: { provider_id: textField(form, "provider_id", "Источник"), display_name: textField(form, "display_name", "Название подключения"), source_company_id: textField(form, "source_company_id", "ID компании у источника") } }, signal), "Подключение создано", formIdentity(form));

        if (result.ok) { setAdding(false); setSelectedConnection(result.data.id); resource.reload(); }
    };


    const sync = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            if (!connection || !provider) { return invalid("Выберите подключение.", "connection"); }
            const from = dateField(form, "period_start", "Начало периода");
            const to = dateField(form, "period_end", "Конец периода");
            const kinds = form.getAll("record_kinds").map(String) as IImportRecordKinds;

            if (from > to) { return invalid("Дата начала не может быть позже окончания.", "period_start"); }
            if (!kinds.length || kinds.some(( kind ) => !provider.capabilities.includes(kind))) { return invalid("Выберите доступные типы данных.", "record_kinds"); }

            return integrationsService.syncConnection({ company_id: company.id, connection_id: connection.id, idempotency_key: key, data: { period_start: from, period_end: to, record_kinds: kinds } }, signal);
        }, "Данные отправлены на проверку", formIdentity(form));

        if (result.ok) { openImport(result.data.id); }
    };


    const disable = async () => {
        if (!disabled) { return; }
        const result = await request.run(( signal ) => integrationsService.disableConnection({ company_id: company.id, connection_id: disabled.id }, signal), "Подключение отключено", disabled.id);

        if (result.ok) { setDisabled(null); setSelectedConnection(""); resource.reload(); }
    };


    return <main className={styles.page}><PageHeading title="Импорт данных" description="Сначала проверка и предварительный просмотр, затем ваше подтверждение. Загрузка сама по себе не меняет операции." /><div className={styles.stack}>{ importId && <ImportJob key={importId} id={importId} onChanged={resource.reload} /> }<Panel title="Добавить данные"><div className={styles.row}><ActionButton variant="secondary" aria-pressed={tab === "file"} disabled={request.busy} onClick={() => setTab("file")}>Из файла</ActionButton><ActionButton variant="secondary" aria-pressed={tab === "external"} disabled={request.busy} onClick={() => setTab("external")}>Из внешнего источника</ActionButton></div><div className={styles.importForm}>{ tab === "file" ? <form noValidate className={styles.form} onSubmit={upload}><fieldset disabled={request.busy}><Field name="file" label="Выписка CSV" type="file" accept=".csv,text/csv" hint="Формат dataset_v1. Файл должен содержать операции в рублях." /><Field name="source_company_id" label="ID компании в файле" placeholder="Значение company_id из выписки" hint="Из общего файла будут выбраны только строки указанной компании." /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Загрузить и проверить</ActionButton></form> : <DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}><form noValidate className={styles.form} onSubmit={sync}><Notice>Доступные внешние источники работают в демонстрационном режиме. Данные поступят на предварительный просмотр.</Notice><fieldset disabled={request.busy}><Select disabled={request.busy} name="connection" label="Подключение" value={selectedConnection} onChange={( event ) => setSelectedConnection(event.target.value)}><option value="">Выберите подключение</option>{ resource.data?.connections.filter(( item ) => item.status === "ready").map(( item ) => <option key={item.id} value={item.id}>{ item.display_name }</option>) }</Select>{ connection && <><Field name="period_start" label="Начало периода" type="date" defaultValue={addDays(today(), -30)} /><Field name="period_end" label="Конец периода" type="date" defaultValue={today()} /><fieldset className={styles.checks}><legend>Какие данные загрузить</legend>{ provider?.capabilities.map(( kind ) => <label key={`${connection.id}-${kind}`}><input type="checkbox" name="record_kinds" value={kind} defaultChecked />{ { transactions: "Фактические операции", planned_payments: "Плановые платежи", balance_observations: "Остатки на счёте" }[kind] }</label>) }</fieldset></> }</fieldset><div className={styles.row}><ActionButton type="submit" loading={request.busy} disabled={!connection}>Получить и проверить</ActionButton><ActionButton variant="secondary" disabled={request.busy || !resource.data?.providers.length} onClick={() => setAdding(true)}>Новое подключение</ActionButton>{ connection && <ActionButton variant="secondary" disabled={request.busy} onClick={() => setDisabled(connection)}>Отключить</ActionButton> }</div><FormError message={request.error} /></form></DataState> }</div></Panel><Panel title="История загрузок"><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data?.history.length ? resource.data.history.map(( item ) => <div className={styles.item} key={item.id}><div><h3>{ item.source_kind === "file" ? "Выписка из файла" : "Внешний источник" }</h3><p className={styles.muted}>{ formatDate(item.created_at) } · { item.preview ? `${formatDate(item.preview.period_start)} — ${formatDate(item.preview.period_end)}` : "Период определяется" }</p><Badge tone={item.status === "failed" ? "danger" : item.status === "applied" ? "success" : "neutral"}>{ importStatus[item.status] }</Badge></div><ActionButton variant="secondary" onClick={() => openImport(item.id)}>Открыть</ActionButton></div>) : <EmptyState title="История пока пуста" icon="file-upload">Загрузите выписку, чтобы проверить и добавить операции.</EmptyState> }</DataState></Panel></div>{ adding && <Modal title="Новое подключение" busy={request.busy} onClose={() => setAdding(false)}><form className={styles.form} noValidate onSubmit={createConnection}><fieldset disabled={request.busy}><Select disabled={request.busy} name="provider_id" label="Источник"><option value="">Выберите источник</option>{ resource.data?.providers.map(( item ) => <option key={item.id} value={item.id}>{ item.name } · демо</option>) }</Select><Field name="display_name" label="Название подключения" placeholder="Например, основной счёт" /><Field name="source_company_id" label="ID компании у источника" /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Создать подключение</ActionButton></form></Modal> }{ disabled && <Modal title="Отключить источник?" busy={request.busy} onClose={() => setDisabled(null)}><p>Подключение «{ disabled.display_name }» перестанет получать новые данные. Уже добавленные операции сохранятся.</p><div className={styles.actions}><ActionButton variant="secondary" disabled={request.busy} onClick={() => setDisabled(null)}>Оставить</ActionButton><ActionButton variant="danger" loading={request.busy} onClick={disable}>Отключить</ActionButton></div><FormError message={request.error} /></Modal> }</main>;
};


const ImportJob = ( { id, onChanged }: { id: string; onChanged: () => void } ) => {
    const { company, reload } = useCompany();
    const [ acknowledged, setAcknowledged ] = useState(false);
    const request = useRequest();
    const resource = useJobResource(useCallback(( signal: AbortSignal ) => importsService.getImport({ company_id: company.id, import_id: id }, signal), [ company.id, id ]));
    const preview = resource.data?.preview;
    const issues = useResource(useCallback(( signal: AbortSignal ) => preview ? collectPages(( cursor ) => importsService.listImportIssues({ company_id: company.id, import_id: id, limit: 100, cursor }, signal)) : Promise.resolve([]), [ company.id, id, preview ]));


    const commit = async () => {
        if (!resource.data || !preview) { return; }
        const version = resource.data.version;
        const result = await request.run(( signal, key ) => {
            if (!preview.can_commit || preview.blocking_issue_count) { return invalid("Исправьте блокирующие ошибки в исходном файле и загрузите его заново."); }
            if (preview.warning_count && !acknowledged) { return invalid("Ознакомьтесь с предупреждениями и подтвердите продолжение."); }

            return importsService.commitImport({ company_id: company.id, import_id: id, idempotency_key: key, data: { version, expected_data_version: preview.base_data_version, acknowledge_warnings: acknowledged } }, signal);
        }, "Данные добавлены в компанию", `${id}:${version}:${acknowledged}`);

        if (result.ok) { resource.reload(); onChanged(); reload(); }
    };


    return <Panel title="Результат проверки"><DataState loading={resource.loading && !resource.data} error={resource.error} onRetry={resource.reload}>{ resource.data && <div className={styles.stack}>{ [ "queued", "processing" ].includes(resource.data.status) ? <JobState label="Проверяем данные" /> : resource.data.status === "failed" ? <Notice tone="danger">{ resource.data.error.message || "Не удалось обработать загрузку. Проверьте файл и попробуйте ещё раз." }</Notice> : preview && <><Notice tone={resource.data.status === "applied" ? "success" : preview.can_commit ? "neutral" : "danger"}>{ resource.data.status === "applied" ? "Загрузка применена. Операции доступны в разделе «Операции и данные»." : preview.can_commit ? "Проверка завершена. Просмотрите данные перед добавлением." : "Загрузка содержит ошибки. Исправьте исходные данные и загрузите файл повторно." }</Notice><div className={styles.grid}><div><span className={styles.muted}>Новые операции</span><p className={styles.stat}>{ preview.new_transaction_count }</p></div><div><span className={styles.muted}>Новые платежи</span><p className={styles.stat}>{ preview.new_payment_count }</p></div><div><span className={styles.muted}>Уже загружены</span><p className={styles.stat}>{ preview.already_present_count }</p></div></div><p>Период: { formatDate(preview.period_start) } — { formatDate(preview.period_end) }. Прочитано строк: { preview.row_count }. Исключено строк других компаний: { preview.excluded_other_company_row_count }.</p>{ preview.balance_observations.length > 0 && <div><h3>Остатки из выписки</h3>{ preview.balance_observations.map(( item, index ) => <p key={index}>{ formatDate(item.date) }: { formatMoney(item.balance.amount_minor) }</p>) }</div> }<p>Ошибок: { preview.blocking_issue_count }. Предупреждений: { preview.warning_count }.</p>{ (preview.blocking_issue_count > 0 || preview.warning_count > 0) && <details><summary className={styles.summary}>Посмотреть все замечания</summary><DataState loading={issues.loading} error={issues.error} onRetry={issues.reload}>{ issues.data?.map(( issue, index ) => <div className={styles.issue} key={index}><Badge tone={issue.severity === "blocking" ? "danger" : "warning"}>{ issue.severity === "blocking" ? "Ошибка" : "Предупреждение" }</Badge><p>{ issue.row_number !== null ? `Строка ${issue.row_number}: ` : "" }{ issue.message }</p></div>) }</DataState></details> }{ resource.data.status === "preview_ready" && <>{ preview.warning_count > 0 && <label className={styles.acknowledgement}><input type="checkbox" checked={acknowledged} disabled={request.busy} onChange={( event ) => setAcknowledged(event.target.checked)} />Я ознакомился с предупреждениями и подтверждаю добавление данных</label> }<ActionButton loading={request.busy} disabled={!preview.can_commit || preview.blocking_issue_count > 0 || (preview.warning_count > 0 && !acknowledged)} onClick={commit}>Подтвердить и добавить данные</ActionButton><FormError message={request.error} /></> }</> }</div> }</DataState></Panel>;
};


export default Imports;
