"use client";

import { useState, type InputHTMLAttributes, type Ref } from "react";

import { useFieldError } from "../ValidatedForm/ValidatedForm";

import styles from "./style.module.scss";


interface IProps extends InputHTMLAttributes<HTMLInputElement> {
    label: string;
    name: string;
    showLabel?: boolean;
    ref?: Ref<HTMLInputElement>;
}


const AuthInput = ( { label, name, id = `auth-${name}`, showLabel = false, type = "text", ...props }: IProps ) => {
    const [ visible, setVisible ] = useState(false);
    const error = useFieldError(name);
    const isPassword = type === "password";
    const passwordLabel = name === "confirm_password" ? "подтверждение пароля" : "пароль";


    return (
        <div className={styles.field}>
            <label className={showLabel ? styles.visibleLabel : styles.label} htmlFor={id}>{ label }</label>
            <div className={styles.control}>
                <input {...props} aria-invalid={Boolean(error) || undefined} aria-describedby={error ? `${id}-error` : props["aria-describedby"]} className={`${styles.input} ${isPassword ? styles.password : ""}`} id={id} name={name} type={isPassword && visible ? "text" : type} placeholder={props.placeholder || label} />
                { error && <span className={styles.label} id={`${id}-error`}>{ error }</span> }

                { isPassword && (
                    <button
                        className={styles.toggle}
                        type="button"
                        aria-label={`${visible ? "Скрыть" : "Показать"} ${passwordLabel}`}
                        aria-controls={id}
                        aria-pressed={visible}
                        disabled={props.disabled}
                        onClick={( event ) => {
                            event.preventDefault();
                            setVisible(( current ) => !current);
                        }}
                    >
                        <span className={`${styles.eye} ${visible ? styles.eyeOff : ""}`} aria-hidden="true" />
                    </button>
                ) }
            </div>
        </div>
    );
};


export default AuthInput;
