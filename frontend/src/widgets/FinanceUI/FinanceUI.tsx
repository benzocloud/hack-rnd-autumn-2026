"use client";

import Dialog from "@mui/material/Dialog";
import MenuItem from "@mui/material/MenuItem";
import MuiSelect, { type SelectProps } from "@mui/material/Select";
import Link from "next/link";
import { Children, isValidElement, useId, type ButtonHTMLAttributes, type CSSProperties, type InputHTMLAttributes, type OptionHTMLAttributes, type ReactElement, type ReactNode, type TextareaHTMLAttributes } from "react";

import AuthButton from "../AuthButton/AuthButton";
import LoadingText from "../LoadingText/LoadingText";
import Spinner from "../Spinner/Spinner";
import { useClearFieldError, useFieldError } from "../ValidatedForm/ValidatedForm";

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
    const error = useFieldError(props.name);

    return <div className={styles.field}><label htmlFor={id}>{ label }</label><input {...props} id={id} aria-invalid={Boolean(error) || undefined} aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined} />{ error && <small className={styles.fieldError} id={`${id}-error`}>{ error }</small> }{ hint && <small id={`${id}-hint`}>{ hint }</small> }</div>;
};


const SelectArrow = ( { className }: { className?: string } ) => (
    <span className={`${className || ""} ${styles.selectArrow}`} aria-hidden="true"><Icon name="chevron-down" /></span>
);


const Select = ( { label, children, hint, className, ...props }: Omit<SelectProps<string>, "children" | "label"> & { label: string; hint?: string; children: ReactNode } ) => {
    const id = useId();
    const error = useFieldError(props.name);
    const clearFieldError = useClearFieldError();
    const options = Children.toArray(children).filter(( child ): child is ReactElement<OptionHTMLAttributes<HTMLOptionElement>> => isValidElement(child) && child.type === "option");

    return (
        <div className={styles.field}>
            <label id={`${id}-label`} htmlFor={id}>{ label }</label>
            <MuiSelect
                {...props}
                onChange={( event, child ) => { if (props.name) { clearFieldError(props.name); } props.onChange?.(event, child); }}
                id={id}
                error={Boolean(error)}
                data-validation-name={props.name}
                labelId={`${id}-label`}
                aria-describedby={hint ? `${id}-hint` : undefined}
                className={`${styles.select} ${className || ""}`}
                defaultValue={props.defaultValue ?? String(options[0]?.props.value ?? "")}
                IconComponent={SelectArrow}
                displayEmpty
                MenuProps={{
                    transitionDuration: 300,
                    slotProps: {
                        paper: { className: styles.selectMenu },
                        list: { className: styles.selectList },
                    },
                }}
            >
                { options.map(( option, index ) => <MenuItem key={option.key ?? index} className={styles.selectOption} value={String(option.props.value ?? "")} disabled={option.props.disabled}>{ option.props.children }</MenuItem>) }
            </MuiSelect>
            { hint && <small id={`${id}-hint`}>{ hint }</small> }
        </div>
    );
};


const TextArea = ( { label, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string } ) => {
    const id = useId();
    const error = useFieldError(props.name);

    return <div className={styles.field}><label htmlFor={id}>{ label }</label><textarea {...props} id={id} rows={3} aria-invalid={Boolean(error) || undefined} aria-describedby={error ? `${id}-error` : undefined} />{ error && <small className={styles.fieldError} id={`${id}-error`}>{ error }</small> }</div>;
};


const Modal = ( { title, children, onClose, busy = false }: IChildren & { title: string; onClose: () => void; busy?: boolean } ) => {
    const id = useId();

    return (
        <Dialog
            open
            maxWidth={false}
            aria-labelledby={id}
            transitionDuration={300}
            onClose={() => { if (!busy) { onClose(); } }}
            slotProps={{ paper: { className: styles.dialog }, backdrop: { className: styles.backdrop } }}
        >
            <div className={styles.modalHeading}>
                <h2 id={id}>{ title }</h2>
                <button type="button" className={styles.close} onClick={onClose} disabled={busy} aria-label="Закрыть окно"><Icon name="x" /></button>
            </div>
            { children }
        </Dialog>
    );
};


export { Icon, ActionButton, LinkButton, PageHeading, Panel, Notice, Badge, EmptyState, DataState, JobState, FormError, Field, Select, TextArea, Modal };
