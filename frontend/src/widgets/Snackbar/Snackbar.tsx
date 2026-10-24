"use client";

import Alert from "@mui/material/Alert";
import MuiSnackbar from "@mui/material/Snackbar";
import Slide, { type SlideProps } from "@mui/material/Slide";
import useMediaQuery from "@mui/material/useMediaQuery";
import { useState } from "react";

import { SnackbarSeverity } from "@/interfaces/Enums";

import styles from "./style.module.scss";


interface IProps {
    message: string;
    onClose: () => void;
}


interface ISnackbarProps extends IProps {
    severity: SnackbarSeverity;
}


const CloseIcon = () => <span aria-hidden="true" className={`${styles.icon} ${styles.close}`} />;


const SnackbarSlide = ( props: SlideProps ) => <Slide {...props} direction="down" />;


const Snackbar = ( { message, severity, onClose }: ISnackbarProps ) => {
    const [ open, setOpen ] = useState(true);
    const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");


    return (
        <MuiSnackbar
            className={styles.snackbar}
            open={open}
            slots={{ transition: SnackbarSlide }}
            slotProps={{ transition: { onExited: onClose } }}
            anchorOrigin={{ vertical: "top", horizontal: "center" }}
            autoHideDuration={6000}
            transitionDuration={reducedMotion ? 0 : 300}
            onClose={( event, reason ) => {
                if (reason !== "clickaway") {
                    setOpen(false);
                }
            }}
        >
            <Alert
                className={styles.alert}
                severity={severity}
                variant="filled"
                role={severity === SnackbarSeverity.Error || severity === SnackbarSeverity.Warning ? "alert" : "status"}
                icon={<span aria-hidden="true" className={`${styles.icon} ${styles[severity]}`} />}
                closeText="Закрыть уведомление"
                slots={{ closeIcon: CloseIcon }}
                onClose={() => setOpen(false)}
            >
                { message }
            </Alert>
        </MuiSnackbar>
    );
};


const WarningSnackbar = ( props: IProps ) => <Snackbar {...props} severity={SnackbarSeverity.Warning} />;


const InfoSnackbar = ( props: IProps ) => <Snackbar {...props} severity={SnackbarSeverity.Info} />;


const ErrorSnackbar = ( props: IProps ) => <Snackbar {...props} severity={SnackbarSeverity.Error} />;


const SuccessSnackbar = ( props: IProps ) => <Snackbar {...props} severity={SnackbarSeverity.Success} />;


export { WarningSnackbar, InfoSnackbar, ErrorSnackbar, SuccessSnackbar };
export default Snackbar;
