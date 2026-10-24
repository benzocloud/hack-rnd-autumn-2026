import { createSlice, type PayloadAction } from "@reduxjs/toolkit";

import type { ISnackbar, ISnackbarState } from "@/interfaces/snackbar";


const initialState: ISnackbarState = { queue: [] };


const snackbarSlice = createSlice({
    name: "snackbar",
    initialState,
    reducers: {
        enqueueSnackbar: ( state, action: PayloadAction<ISnackbar> ) => {
            if (state.queue.some(( item ) => item.message === action.payload.message && item.severity === action.payload.severity)) {
                return;
            }

            state.queue.push(action.payload);
        },
        dismissSnackbar: ( state, action: PayloadAction<string> ) => {
            state.queue = state.queue.filter(( item ) => item.id !== action.payload);
        },
    },
});


const { enqueueSnackbar, dismissSnackbar } = snackbarSlice.actions;


export { enqueueSnackbar, dismissSnackbar };
export default snackbarSlice.reducer;
