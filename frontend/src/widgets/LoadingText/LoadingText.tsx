import type { ReactNode } from "react";

import styles from "./style.module.scss";


interface IProps {
    children: ReactNode;
}


const LoadingText = ( { children }: IProps ) => {
    return <span>{ children }<span className={styles.dots} aria-hidden="true">...</span></span>;
};


export default LoadingText;
