"use client";

import { useState, type FormEvent } from "react";

import { useCompany } from "@/features/CompanyShell/CompanyShell";
import { dateField, formatDate, formIdentity, invalid, moneyInput, parseMoney, textField, today } from "@/helpers/finance";
import formatMoney from "@/helpers/formatMoney";
import useRequest from "@/hooks/useRequest";
import { companiesService, ledgerService } from "@/service";
import { ActionButton, Field, FormError, Modal, Notice, PageHeading, Panel, TextArea } from "@/widgets/FinanceUI/FinanceUI";

import ValidatedForm from "@/widgets/ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


const CompanySettings = () => {
    const { company, account, reload } = useCompany();
    const [ editing, setEditing ] = useState(false);
    const [ balanceOpen, setBalanceOpen ] = useState(false);
    const request = useRequest();
    const owner = company.membership.role === "owner";


    const saveCompany = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal ) => {
            const name = textField(form, "name", "Название компании");
            const inn = textField(form, "inn", "ИНН", 12, true);

            if (inn && !/^([0-9]{10}|[0-9]{12})$/.test(inn)) {
                invalid("ИНН должен содержать 10 или 12 цифр.", "inn");
            }

            return companiesService.updateCompany({ company_id: company.id, data: { name, inn: inn || null, version: company.version } }, signal);
        }, "Данные компании сохранены", formIdentity(form));

        if (result.ok) {
            setEditing(false);
            reload();
        }
    };


    const saveBalance = async ( event: FormEvent<HTMLFormElement> ) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const result = await request.run(( signal, key ) => {
            const asOf = dateField(form, "as_of", "Дата остатка");

            if (asOf > today()) {
                invalid("Остаток можно указать только на текущую или прошедшую дату.", "as_of");
            }

            return ledgerService.setDatedBalance({ company_id: company.id, idempotency_key: key, data: { balance: { amount_minor: parseMoney(textField(form, "amount", "Остаток"), true), currency: "RUB" }, as_of: asOf, expected_data_version: account.data_version, note: textField(form, "note", "Основание", 1000) } }, signal);
        }, "Остаток сохранён. Для актуальных данных постройте новый прогноз.", formIdentity(form));

        if (result.ok) {
            setBalanceOpen(false);
            reload();
        }
    };


    return <main className={styles.page}><PageHeading title="Компания и счёт" description="Реквизиты компании и исходная точка для финансового прогноза." /><div className={styles.grid}><Panel title="Данные компании" actions={owner && <ActionButton variant="secondary" icon="pencil" onClick={() => setEditing(true)}>Изменить</ActionButton>}><h3>{ company.name }</h3><p>ИНН: { company.inn || "Не указан" }</p><p>Российский рубль · Москва (UTC+3)</p>{ !owner && <Notice>Реквизиты компании может менять только владелец.</Notice> }</Panel><Panel title="Остаток на счёте"><div className={styles.balance}><p className={styles.stat}>{ account.balance ? formatMoney(account.balance.amount_minor) : "Не указан" }</p><p className={styles.muted}>{ account.balance_as_of ? `На конец дня ${formatDate(account.balance_as_of)}` : "Укажите сумму и дату, чтобы построить первый прогноз." }</p><p className={styles.muted}>Ручной остаток — отдельный датированный якорь. Он не вычисляет баланс по операциям и не подтверждает неизвестные дни. Изменение остатка не создаёт операцию. Существующие прогнозы сохраняются, но могут стать неактуальными.</p><ActionButton variant="secondary" onClick={() => setBalanceOpen(true)}>{ account.balance ? "Обновить остаток" : "Указать остаток" }</ActionButton></div></Panel></div>{ editing && <Modal title="Данные компании" busy={request.busy} onClose={() => setEditing(false)}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={saveCompany}><fieldset disabled={request.busy}><Field required data-maximum="200" name="name" label="Название компании" defaultValue={company.name} autoFocus /><Field data-maximum="12" name="inn" label="ИНН — необязательно" inputMode="numeric" defaultValue={company.inn || ""} /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Сохранить изменения</ActionButton></ValidatedForm></Modal> }{ balanceOpen && <Modal title="Остаток на конец дня" busy={request.busy} onClose={() => setBalanceOpen(false)}><ValidatedForm request={request} className={styles.form} noValidate onSubmit={saveBalance}><fieldset disabled={request.busy}><Field required data-maximum="200" data-allow-negative="true" name="amount" label="Остаток, ₽" inputMode="decimal" defaultValue={account.balance ? moneyInput(account.balance.amount_minor) : ""} placeholder="0,00" autoFocus /><Field required data-maximum="200" name="as_of" label="Дата остатка" type="date" defaultValue={account.balance_as_of || today()} /><TextArea required data-maximum="1000" name="note" label="Основание для изменения" placeholder="Например, остаток из банковской выписки" /></fieldset><FormError message={request.error} /><ActionButton type="submit" loading={request.busy}>Сохранить остаток</ActionButton></ValidatedForm></Modal> }</main>;
};


export default CompanySettings;
