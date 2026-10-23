import { SnackbarSeverity } from "@/interfaces/Enums";
import type { ISnackbar } from "@/interfaces/snackbar";


const listeners = new Set<( snackbar: ISnackbar ) => void>();

const pending: ISnackbar[] = [];


const showSnackbar = ( message: string, severity: SnackbarSeverity ) => {
    if (typeof window === "undefined") {
        return;
    }

    const snackbar: ISnackbar = { id: crypto.randomUUID(), message, severity };

    if (!listeners.size) {
        pending.push(snackbar);

        return;
    }

    listeners.forEach(( listener ) => listener(snackbar));
};


const subscribeToSnackbars = ( listener: ( snackbar: ISnackbar ) => void ) => {
    listeners.add(listener);
    pending.splice(0).forEach(listener);

    return () => {
        listeners.delete(listener);
    };
};


const showWarningSnackbar = ( message: string ) => showSnackbar(message, SnackbarSeverity.Warning);


const showInfoSnackbar = ( message: string ) => showSnackbar(message, SnackbarSeverity.Info);


const showErrorSnackbar = ( message: string ) => showSnackbar(message, SnackbarSeverity.Error);


const showSuccessSnackbar = ( message: string ) => showSnackbar(message, SnackbarSeverity.Success);


export { subscribeToSnackbars, showWarningSnackbar, showInfoSnackbar, showErrorSnackbar, showSuccessSnackbar };
