import styles from "./style.module.scss";


interface IProps {
    label?: string;
}


const Spinner = ( { label = "Загрузка" }: IProps ) => {
    return <span className={styles.spinner} role="status" aria-label={label}><span className={styles.circle} aria-hidden="true" /></span>;
};


export default Spinner;
