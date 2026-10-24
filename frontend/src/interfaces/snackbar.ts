import type { SnackbarSeverity } from "./Enums";


interface ISnackbar {
    id: string;
    message: string;
    severity: SnackbarSeverity;
}


interface ISnackbarState {
    queue: ISnackbar[];
}


export type { ISnackbar, ISnackbarState };
