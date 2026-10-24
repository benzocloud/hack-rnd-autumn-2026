"use client";

import Link from "next/link";
import { useEffect, useId, useRef, type ButtonHTMLAttributes, type CSSProperties, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";

import AuthButton from "../AuthButton/AuthButton";
import LoadingText from "../LoadingText/LoadingText";
import Spinner from "../Spinner/Spinner";

import styles from "./style.module.scss";


interface IChildren { children: ReactNode; }


const Icon = ( { name, size = 20 }: { name: string; size?: number } ) => <span className={styles.icon} style={{ "--icon": `url("/icons/app/${name}.svg")`, width: size, height: size } as CSSProperties} aria-hidden="true" />;


const ActionButton = ( { variant = "primary", loading = false, icon, children, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger"; loading?: boolean; icon?: string } ) => <AuthButton {...props} loading={loading} className={`${styles.action} ${styles[variant]}`}><span className={styles.buttonContent}>{ icon && <Icon name={icon} /> }{ children !== undefined && <span className={styles.buttonLabel}>{ children }</span> }</span></AuthButton>;


const LinkButton = ( { href, children, icon, secondary = false }: IChildren & { href: string; icon?: string; secondary?: boolean } ) => <Link href={href} className={`${styles.action} ${secondary ? styles.secondary : styles.primary}`}><span className={styles.buttonContent}>{ icon && <Icon name={icon} /> }{ children !== undefined && <span className={styles.buttonLabel}>{ children }</span> }</span></Link>;


const PageHeading = ( { title, description, children }: { title: string; description?: string; children?: ReactNode } ) => <div className={styles.heading}><div><h1>{ title }</h1>{ description && <p>{ description }</p> }</div>{ children && <div className={styles.actions}>{ children }</div> }</div>;


const Panel = ( { title, children, actions }: IChildren & { title?: string; actions?: ReactNode } ) => <section className={styles.panel}>{ (title || actions) && <div className={styles.panelHeading}>{ title && <h2>{ title }</h2> }{ actions }</div> }{ children }</section>;


const Notice = ( { children, tone = "neutral" }: IChildren & { tone?: "neutral" | "warning" | "danger" | "success" } ) => <div className={`${styles.notice} ${styles[tone]}`}><Icon name={tone === "success" ? "circle-check" : tone === "neutral" ? "info-circle" : "alert-triangle"} /><div>{ children }</div></div>;


const Badge = ( { children, tone = "neutral" }: IChildren & { tone?: "neutral" | "warning" | "danger" | "success" } ) => <span className={`${styles.badge} ${styles[tone]}`}>{ children }</span>;


const EmptyState = ( { title, children, icon = "folder", action }: { title: string; children?: ReactNode; icon?: string; action?: ReactNode } ) => <div className={styles.empty}><span className={styles.emptyIcon}><Icon name={icon} size={28} /></span><h2>{ title }</h2>{ children && <p>{ children }</p> }{ action }</div>;


const DataState = ( { loading, error, onRetry, children }: { loading: boolean; error: string | null; onRetry: () => void; children: ReactNode } ) => error ? <EmptyState title="Не удалось загрузить данные" icon="alert-triangle" action={<ActionButton variant="secondary" icon="refresh" onClick={onRetry}>Повторить</ActionButton>}>{ error }</EmptyState> : loading ? <div className={styles.loading} role="status"><Spinner /><p>Загружаем данные</p></div> : children;


const JobState = ( { label }: { label: string } ) => <div className={styles.loading} role="status"><Spinner /><p><LoadingText>{ label }</LoadingText></p><small>Можно вернуться позже — принятое задание продолжит работу.</small></div>;


const FormError = ( { message }: { message: string | null } ) => message ? <p className={styles.formError} role="alert">{ message }</p> : null;


const Field = ( { label, hint, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string } ) => {
    const id = useId();

    return <div className={styles.field}><label htmlFor={id}>{ label }</label><input {...props} id={id} aria-describedby={hint ? `${id}-hint` : undefined} />{ hint && <small id={`${id}-hint`}>{ hint }</small> }</div>;
};


const Select = ( { label, children, hint, ...props }: SelectHTMLAttributes<HTMLSelectElement> & { label: string; hint?: string } ) => {
    const id = useId();

    return <div className={styles.field}><label htmlFor={id}>{ label }</label><select {...props} id={id} aria-describedby={hint ? `${id}-hint` : undefined}>{ children }</select>{ hint && <small id={`${id}-hint`}>{ hint }</small> }</div>;
};


const TextArea = ( { label, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string } ) => {
    const id = useId();

    return <div className={styles.field}><label htmlFor={id}>{ label }</label><textarea {...props} id={id} rows={3} /></div>;
};


const Modal = ( { title, children, onClose, busy = false }: IChildren & { title: string; onClose: () => void; busy?: boolean } ) => {
    const ref = useRef<HTMLDialogElement>(null);
    const id = useId();


    useEffect(() => {
        const dialog = ref.current;

        dialog?.showModal();

        return () => dialog?.close();
    }, []);


    return <dialog ref={ref} className={styles.dialog} aria-labelledby={id} onCancel={( event ) => { event.preventDefault(); if (!busy) { onClose(); } }} onClick={( event ) => { if (event.target === event.currentTarget && !busy) { onClose(); } }}><div className={styles.modalHeading}><h2 id={id}>{ title }</h2><button type="button" className={styles.close} onClick={onClose} disabled={busy} aria-label="Закрыть окно"><Icon name="x" /></button></div>{ children }</dialog>;
};


export { Icon, ActionButton, LinkButton, PageHeading, Panel, Notice, Badge, EmptyState, DataState, JobState, FormError, Field, Select, TextArea, Modal };
