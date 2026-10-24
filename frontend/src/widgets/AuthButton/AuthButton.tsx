import type { ButtonHTMLAttributes } from "react";

import Spinner from "../Spinner/Spinner";

import styles from "./style.module.scss";


interface IProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    loading?: boolean;
}


const AuthButton = ( { className = "", loading = false, disabled, children, ...props }: IProps ) => {
    return (
        <button {...props} className={`${styles.button} ${className}`} disabled={disabled || loading} aria-busy={loading || undefined}>
            <span className={`${styles.content} ${loading ? styles.contentHidden : ""}`} aria-hidden={loading || undefined}>{ children }</span>
            { loading && <span className={styles.loading}><Spinner /></span> }
        </button>
    );
};


export default AuthButton;
