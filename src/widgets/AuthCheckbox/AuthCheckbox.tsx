"use client";

import { useState, type InputHTMLAttributes, type ReactNode, type Ref } from "react";

import styles from "./style.module.scss";


interface IProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "children"> {
    children: ReactNode;
    ref?: Ref<HTMLInputElement>;
}


const AuthCheckbox = ( { children, onChange, ...props }: IProps ) => {
    const [ animated, setAnimated ] = useState(false);


    return (
        <label className={styles.checkbox}>
            <input
                {...props}
                className={`${styles.input} ${animated ? styles.animated : ""}`}
                type="checkbox"
                onChange={( event ) => {
                    setAnimated(true);
                    onChange?.(event);
                }}
            />
            <span>{ children }</span>
        </label>
    );
};


export default AuthCheckbox;
