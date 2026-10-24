import type { CSSProperties } from "react";

import styles from "./style.module.scss";


const Icon = ( { name, size = 20 }: { name: string; size?: number } ) => (
    <span className={styles.icon} style={{ "--icon": `url("/icons/app/${name}.svg")`, width: size, height: size } as CSSProperties} aria-hidden="true" />
);


export default Icon;
