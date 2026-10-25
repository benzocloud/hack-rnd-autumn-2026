"use client";

import { useRouter } from "next/navigation";
import { useCallback, useRef, useState, type ChangeEvent, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { addDays, companyPath, dateField, formatDate, formIdentity, invalid, textField, today } from "@/helpers/finance";
import { getErrorDetailMessage } from "@/helpers/getErrorMessage";
import formatMoney from "@/helpers/formatMoney";
import { collectPages } from "@/helpers/pagination";
import useJobResource from "@/hooks/useJobResource";
import useRequest from "@/hooks/useRequest";
import useResource from "@/hooks/useResource";
import type { IConnection, IImportRecordKinds } from "@/interfaces";
import { importsService, integrationsService } from "@/service";
import { ActionButton, Badge, DataState, EmptyState, Field, FormError, Icon, JobState, Modal, Notice, PageHeading, Panel, Select } from "@/widgets/FinanceUI/FinanceUI";

import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


const importStatus = { queued: "В очереди", processing: "Проверяется", preview_ready: "Ожидает подтверждения", applied: "Данные добавлены", failed: "Ошибка" };


type FileCompany = { id: string; rows: number; from: string; to: string };


const rowCount = ( count: number ) => {
    const mod10 = count % 10;
    const mod100 = count % 100;
    const word = mod10 === 1 && mod100 !== 11 ? "строка" : mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) ? "строки" : "строк";

    return `${count} ${word}`;
};


// Read logical CSV records: quoted purposes may contain semicolons and line breaks.
function* csvRecords( text: string ): Generator<string[]> {
    let cells: string[] = [];
    let value = "";
    let quoted = false;
    for (let index = 0; index < text.length; index++) {
        const char = text[index];
        if (char === "\"") {
            if (quoted && text[index + 1] === "\"") { value += "\""; index++; }
            else { quoted = !quoted; }
        } else if (!quoted && char === ";") { cells.push(value); value = ""; }
        else if (!quoted && (char === "\n" || char === "\r")) {
            if (char === "\r" && text[index + 1] === "\n") { index++; }
            cells.push(value);
            if (cells.some(( cell ) => cell.trim())) { yield cells; }
            cells = []; value = "";
        } else { value += char; }
    }
    if (quoted) { throw new Error("В CSV не закрыта кавычка. Исправьте файл перед загрузкой."); }
    cells.push(value);
    if (cells.some(( cell ) => cell.trim())) { yield cells; }
}


const companiesInFile = async ( file: File ): Promise<FileCompany[]> => {
    const records = csvRecords((await file.text()).replace(/^\uFEFF/, ""));
    const header = records.next().value || [];
    const company = header.indexOf("company_id");
    const date = header.indexOf("operation_date");

    if (company < 0) { throw new Error("Это не выписка dataset_v1: нет столбца company_id."); }

    const found = new Map<string, FileCompany>();

    for (const cells of records) {
        const id = cells[company]?.trim() || "";

        if (!id) { continue; }

        const day = date < 0 ? "" : (cells[date] || "").trim().slice(0, 10);
        const current = found.get(id) || { id, rows: 0, from: day, to: day };

        current.rows += 1;
        if (day && (!current.from || day < current.from)) { current.from = day; }
        if (day && day > current.to) { current.to = day; }
        found.set(id, current);
    }

    return [ ...found.values() ];
};


const Imports = ( { importId = "" }: { importId?: string } ) => {
    const { company, account } = useCompany();
    const router = useRouter();
    const [ tab, setTab ] = useState<"file" | "external">("file");
    const [ adding, setAdding ] = useState(false);
    const [ disabled, setDisabled ] = useState<IConnection | null>(null);
    const [ selectedConnection, setSelectedConnection ] = useState("");
    const [ fileCompanies, setFileCompanies ] = useState<FileCompany[]>([]);
    const [ fileCompany, setFileCompany ] = useState("");
    const [ fileProblem, setFileProblem ] = useState("");
    const [ uploadProgress, setUploadProgress ] = useState<number | null>(null);
    const [ readingFile, setReadingFile ] = useState(false);
    const [ fileText, setFileText ] = useState("");
    const [ fileOpen, setFileOpen ] = useState(false);
    const readGeneration = useRef(0);
    const request = useRequest();
    const resource = useResource(useCallback(async ( signal: AbortSignal ) => {
        const history = await collectPages(( cursor ) => importsService.listImports({ company_id: company.id, limit: 100, cursor }, signal));

        return { history };
    }, [ company.id ]));
    const external = useResource(useCallback(async ( signal: AbortSignal ) => {
        if (tab !== "external") {
            return { providers: [], connections: [] };
        }

        const [ providers, connections ] = await Promise.all([
            collectPages(( cursor ) => integrationsService.listExternalProviders({ company_id: company.id, limit: 100, cursor }, signal)),
            collectPages(( cursor ) => integrationsService.listConnections({ company_id: company.id, limit: 100, cursor }, signal)),
        ]);

        return { providers, connections };
    }, [ company.id, tab ]));
    const connection = external.data?.connections.find(( item ) => item.id === selectedConnection);
    const provider = external.data?.providers.find(( item ) => item.id === connection?.provider_id);


    const openImport = ( id: string ) => router.push(`${companyPath(company.id, "imports")}?import=${encodeURIComponent(id)}`, { scroll: false });


    const readFile = async ( event: ChangeEvent<HTMLInputElement> ) => {
        const generation = ++readGeneration.current;
        const file = event.target.files?.[0];

        setFileCompanies([]);
        setFileCompany("");
        setFileProblem("");
        setFileText("");
        setFileOpen(false);
        setUploadProgress(null);
        if (!file?.size) { return; }
        if (file.size > 100 * 1024 * 1024) { setFileProblem("Размер CSV не должен превышать 100 МиБ."); return; }
        setReadingFile(true);

        try {
            const found = await companiesInFile(file);
            const text = await file.text();

            if (generation !== readGeneration.current) { return; }
            if (!found.length) { setFileProblem("В файле нет операций."); return; }
            setFileText(text.replace(/^\uFEFF/, "").slice(0, 20000));

            const bound = account.transaction_source?.mode === "external" ? account.transaction_source.source_company_id : "";

            setFileCompanies(found);
            setFileCompany(found.length === 1 ? found[0].id : found.find(( item ) => item.id === bound)?.id || "");
        } catch (cause) {
            if (generation !== readGeneration.current) { return; }
            setFileProblem(cause instanceof Error ? cause.message : "Не удалось прочитать файл.");
        } finally {
            if (generation === readGeneration.current) { setReadingFile(false); }
        }
    };


    const upload = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const file = form.get("file");

            if (!(file instanceof File) || !file.size) { return invalid("Выберите непустой CSV-файл.", "file"); }
            if (file.size > 100 * 1024 * 1024) { return invalid("Размер CSV не должен превышать 100 МиБ.", "file"); }
            if (!file.name.toLowerCase().endsWith(".csv")) { return invalid("Поддерживаются выписки в формате CSV (dataset_v1).", "file"); }
            if (readingFile) { return invalid("Файл ещё читается.", "file"); }
            if (fileProblem) { return invalid(fileProblem, "file"); }
            if (!fileCompany) { return invalid(fileCompanies.length > 1 ? "Выберите компанию из файла." : "Выберите непустой CSV-файл.", fileCompanies.length > 1 ? "source_company_id" : "file"); }

            return importsService.uploadImport({ company_id: company.id, idempotency_key: key, data: { file, adapter: "dataset_v1", source_company_id: fileCompany, account_id: account.id, currency: "RUB" } }, signal, setUploadProgress);
        }, "Файл принят на проверку", formIdentity(form));

        if (result.ok) { openImport(result.data.id); }
    };


    const createConnection = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => integrationsService.createConnection({ company_id: company.id, idempotency_key: key, data: { provider_id: textField(form, "provider_id", "Источник"), display_name: textField(form, "display_name", "Название подключения"), source_company_id: textField(form, "source_company_id", "ID компании у источника") } }, signal), "Подключение создано", formIdentity(form));

        if (result.ok) { setAdding(false); setSelectedConnection(result.data.id); external.reload(); }
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

        if (result.ok) { setDisabled(null); setSelectedConnection(""); external.reload(); }
    };


    return <main className={styles.page}><PageHeading title="Импорт данных" description="Сначала проверка и предварительный просмотр, затем ваше подтверждение. Загрузка сама по себе не меняет операции." /><div className={styles.stack}>{ importId && <ImportJob key={importId} id={importId} onChanged={resource.reload} /> }<Panel title="Добавить данные"><span id="upload-import" /><div className={styles.sources}><button type="button" className={styles.choice} aria-pressed={tab === "file"} disabled={request.busy} onClick={() => setTab("file")}>Из файла</button><button type="button" className={styles.choice} aria-pressed={tab === "external"} disabled={request.busy} onClick={() => setTab("external")}>Из внешнего источника</button></div><div className={styles.importForm}>{ tab === "file" ? <ValidatedForm request={request} noValidate className={styles.form} onSubmit={upload}><fieldset disabled={request.busy}><Field data-maximum="200" name="file" label="Выписка CSV" type="file" accept=".csv,text/csv" hint="Формат dataset_v1. Файл должен содержать операции в рублях." onChange={readFile} />{ fileCompanies.length > 1 && <Select disabled={request.busy} name="source_company_id" label="Компания в файле" value={fileCompany} onChange={( event ) => setFileCompany(event.target.value)} hint="Будут загружены только строки выбранной компании."><option value="">Выберите компанию</option>{ fileCompanies.map(( item ) => <option key={item.id} value={item.id}>{ item.id } · { rowCount(item.rows) }{ item.from ? ` · ${formatDate(item.from)} — ${formatDate(item.to)}` : "" }</option>) }</Select> }{ fileText && <div><div className={fileOpen ? styles.fileFull : styles.filePreview}>{ fileCompanies.length === 1 && <p>В файле одна компания, код { fileCompanies[0].id }.</p> }<p>Показан фрагмент файла, до 20 000 символов. На проверку отправляется исходный файл целиком.</p><pre>{ fileText }</pre></div>{ !fileOpen && <button type="button" className={styles.more} onClick={() => setFileOpen(true)}>Развернуть фрагмент <Icon name="chevron-down" size={16} /></button> }</div> }</fieldset><FormError message={request.error || fileProblem || null} />{ request.busy && <p role="status">{ uploadProgress === null ? "Отправляем файл…" : uploadProgress < 100 ? `Отправлено ${uploadProgress}%` : "Файл отправлен. Ожидаем подтверждение сервиса…" }</p> }<ActionButton type="submit" loading={request.busy} disabled={readingFile}>Загрузить и проверить</ActionButton></ValidatedForm> : <DataState loading={external.loading} error={external.error} onRetry={external.reload}><ValidatedForm request={request} noValidate className={styles.form} onSubmit={sync}><Notice>Доступные внешние источники работают в демонстрационном режиме. Данные поступят на предварительный просмотр.</Notice><fieldset disabled={request.busy}><Select disabled={request.busy} name="connection" label="Подключение" value={selectedConnection} onChange={( event ) => setSelectedConnection(event.target.value)}><option value="">Выберите подключение</option>{ external.data?.connections.filter(( item ) => item.status === "ready").map(( item ) => <option key={item.id} value={item.id}>{ item.display_name }</option>) }</Select>{ connection && <><Field data-maximum="200" name="period_start" label="Начало периода" type="date" defaultValue={addDays(today(), -30)} /><Field data-maximum="200" name="period_end" label="Конец периода" type="date" defaultValue={today()} /><fieldset className={styles.checks}><legend>Какие данные загрузить</legend>{ provider?.capabilities.map(( kind ) => <label key={`${connection.id}-${kind}`}><input type="checkbox" name="record_kinds" value={kind} defaultChecked />{ { transactions: "Фактические операции", planned_payments: "Плановые платежи", balance_observations: "Остатки на счёте" }[kind] }</label>) }</fieldset></> }</fieldset><div className={styles.row}><ActionButton type="submit" loading={request.busy} disabled={!connection}>Получить и проверить</ActionButton><ActionButton variant="secondary" disabled={request.busy || !external.data?.providers.length} onClick={() => setAdding(true)}>Новое подключение</ActionButton>{ connection && <ActionButton variant="secondary" disabled={request.busy} onClick={() => setDisabled(connection)}>Отключить</ActionButton> }</div><FormError message={request.error} /></ValidatedForm></DataState> }</div></Panel><Panel title="История загрузок"><DataState loading={resource.loading} error={resource.error} onRetry={resource.reload}>{ resource.data?.history.length ? resource.data.history.map(( item ) => <div className={styles.item} key={item.id}><div><h3>{ item.source_kind === "file" ? "Выписка из файла" : "Внешний источник" }</h3><p className={styles.muted}>{ formatDate(item.created_at) } · { item.preview ? `${formatDate(item.preview.period_start)} — ${formatDate(item.preview.period_end)}` : "Период определяется" }</p><Badge tone={item.status === "failed" ? "danger" : item.status === "applied" ? "success" : "neutral"}>{ importStatus[item.status] }</Badge></div><ActionButton variant="secondary" onClick={() => openImport(item.id)}>Открыть</ActionButton></div>) : <EmptyState title="История пока пуста" icon="file-upload">Загрузите выписку, чтобы проверить и добавить операции.</EmptyState> }</DataState></Panel></div>{ adding && <Modal title="Новое подключение" busy={request.busy} onClose={() => setAdding(false)}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={createConnection}><fieldset disabled={request.busy}><Select disabled={request.busy} name="provider_id" label="Источник"><option value="">Выберите источник</option>{ external.data?.providers.map(( item ) => <option key={item.id} value={item.id}>{ item.name } · демо</option>) }</Select><Field data-maximum="200" name="display_name" label="Название подключения" placeholder="Например, основной счёт" /><Field required data-maximum="200" name="source_company_id" label="ID компании у источника" /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Создать подключение</ActionButton></ValidatedForm></Modal> }{ disabled && <Modal title="Отключить источник?" busy={request.busy} onClose={() => setDisabled(null)}><p>Подключение «{ disabled.display_name }» перестанет получать новые данные. Уже добавленные операции сохранятся.</p><div className={styles.actions}><ActionButton variant="secondary" disabled={request.busy} onClick={() => setDisabled(null)}>Оставить</ActionButton><ActionButton variant="danger" loading={request.busy} onClick={disable}>Отключить</ActionButton></div><FormError message={request.error} /></Modal> }</main>;
};


const ImportJob = ( { id, onChanged }: { id: string; onChanged: () => void } ) => {
    const { company, reload } = useCompany();
    const [ acknowledged, setAcknowledged ] = useState(false);
    const [ issuesOpen, setIssuesOpen ] = useState(false);
    const request = useRequest();
    const resource = useJobResource(useCallback(( signal: AbortSignal ) => importsService.getImport({ company_id: company.id, import_id: id }, signal), [ company.id, id ]));
    const preview = resource.data?.preview;
    const stale = request.errorCode === "IMPORT_PREVIEW_STALE";


    const commit = async () => {
        if (!resource.data || !preview || stale) { return; }
        const version = resource.data.version;
        const result = await request.run(( signal, key ) => {
            if (!preview.can_commit || preview.blocking_issue_count) { return invalid("Исправьте блокирующие ошибки в исходном файле и загрузите его заново."); }
            if (preview.warning_count && !acknowledged) { return invalid("Ознакомьтесь с предупреждениями и подтвердите продолжение."); }

            return importsService.commitImport({ company_id: company.id, import_id: id, idempotency_key: key, data: { version, expected_data_version: preview.base_data_version, acknowledge_warnings: acknowledged } }, signal);
        }, "Данные добавлены в компанию", `${id}:${version}:${acknowledged}`);

        if (result.ok) { resource.reload(); onChanged(); reload(); }
    };


    return <Panel title="Результат проверки"><DataState loading={resource.loading && !resource.data} error={resource.error} onRetry={resource.reload}>{ resource.data && <div className={styles.stack}>{ [ "queued", "processing" ].includes(resource.data.status) ? <JobState label="Проверяем данные" /> : resource.data.status === "failed" ? <Notice tone="danger">{ getErrorDetailMessage(resource.data.error) }</Notice> : preview && <><Notice tone={resource.data.status === "applied" ? "success" : preview.blocking_issue_count ? "danger" : "neutral"}>{ resource.data.status === "applied" ? "Загрузка применена. Операции доступны в разделе «Операции и данные»." : !preview.blocking_issue_count ? "Проверка завершена. Просмотрите загруженные данные." : "Загрузка содержит ошибки. Исправьте исходные данные и загрузите файл повторно." }</Notice><div className={styles.grid}><div><span className={styles.muted}>Новые операции</span><p className={styles.stat}>{ preview.new_transaction_count }</p></div><div><span className={styles.muted}>Новые платежи</span><p className={styles.stat}>{ preview.new_payment_count }</p></div><div><span className={styles.muted}>Уже загружены</span><p className={styles.stat}>{ preview.already_present_count }</p></div></div><p>Период: { formatDate(preview.period_start) } — { formatDate(preview.period_end) }. Прочитано строк: { preview.row_count }. Исключено строк других компаний: { preview.excluded_other_company_row_count }.</p>{ resource.data.expires_at && <p>Предварительный просмотр доступен до { formatDate(resource.data.expires_at.slice(0, 10)) }.</p> }{ preview.history_coverage?.kind === "dataset_fixture_assumption" && <p>{ resource.data.status === "applied" ? "Полнота истории принята по условиям демонстрационного набора, а не подтверждена банком." : "Для этого демонстрационного набора настроено допущение полноты истории; оно будет применено только после успешного commit и проверки всех операций счёта." }</p> }{ resource.data.status === "preview_ready" && !preview.can_commit && preview.blocking_issue_count === 0 && <p>Применение данных пока недоступно. Загруженные операции ещё не добавлены на счёт.</p> }{ preview.balance_observations.length > 0 && <div><h3>Остатки из выписки</h3>{ preview.balance_observations.map(( item, index ) => <p key={index}>{ formatDate(item.date) }: { formatMoney(item.balance.amount_minor) }</p>) }</div> }<p>Ошибок: { preview.blocking_issue_count }. Предупреждений: { preview.warning_count }.</p>{ (preview.blocking_issue_count > 0 || preview.warning_count > 0) && <details onToggle={( event ) => setIssuesOpen(event.currentTarget.open)}><summary className={styles.summary}>Посмотреть замечания ({ preview.blocking_issue_count + preview.warning_count })</summary>{ issuesOpen && <ImportIssues key={preview.preview_version} companyId={company.id} importId={id} total={preview.blocking_issue_count + preview.warning_count} /> }</details> }{ resource.data.status === "preview_ready" && <>{ preview.warning_count > 0 && <label className={styles.acknowledgement}><input type="checkbox" checked={acknowledged} disabled={request.busy} onChange={( event ) => setAcknowledged(event.target.checked)} />Я ознакомился с предупреждениями и подтверждаю добавление данных. При совпадении с ручной операцией обе записи сохранятся; автоматического объединения нет.</label> }{ stale && <Notice tone="warning">Проверка устарела. <a href="#upload-import">Загрузить и проверить файл заново</a>. Новое применение потребует отдельного подтверждения.</Notice> }<ActionButton loading={request.busy} disabled={stale || !preview.can_commit || preview.blocking_issue_count > 0 || (preview.warning_count > 0 && !acknowledged)} onClick={commit}>Подтвердить и добавить данные</ActionButton><FormError message={request.error} /></> }</> }</div> }</DataState></Panel>;
};


const ImportIssues = ( { companyId, importId, total }: { companyId: string; importId: string; total: number } ) => {
    const [ cursors, setCursors ] = useState<string[]>([]);
    const cursor = cursors.at(-1);
    const issues = useResource(useCallback(( signal: AbortSignal ) => importsService.listImportIssues({ company_id: companyId, import_id: importId, limit: 100, cursor }, signal), [ companyId, importId, cursor ]));


    return <DataState loading={issues.loading} error={issues.error} onRetry={issues.reload}>{ issues.data && <><p>Страница { cursors.length + 1 }. На странице: { issues.data.items.length }. Всего замечаний: { total }.</p>{ issues.data.items.map(( issue, index ) => <div className={styles.issue} key={index}><Badge tone={issue.severity === "blocking" ? "danger" : "warning"}>{ issue.severity === "blocking" ? "Ошибка" : "Предупреждение" }</Badge><p>{ issue.row_number !== null ? `Строка ${issue.row_number}: ` : "" }{ issue.message }</p></div>) }<div className={styles.row}><ActionButton variant="secondary" disabled={!cursors.length} onClick={() => setCursors(( current ) => current.slice(0, -1))}>Назад</ActionButton><ActionButton variant="secondary" disabled={!issues.data.next_cursor} onClick={() => { const next = issues.data?.next_cursor; if (next) { setCursors(( current ) => [ ...current, next ]); } }}>Далее</ActionButton></div></> }</DataState>;
};


export default Imports;
