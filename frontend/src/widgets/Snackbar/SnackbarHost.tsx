"use client";

import { useEffect } from "react";
import { useDispatch, useSelector } from "react-redux";

import { subscribeToSnackbars } from "@/helpers/snackbar";
import type { AppDispatch, RootState } from "@/store/store";
import { dismissSnackbar, enqueueSnackbar } from "@/store/snackbarSlice";

import Snackbar from "./Snackbar";


const SnackbarHost = () => {
    const dispatch = useDispatch<AppDispatch>();

    const snackbar = useSelector(( state: RootState ) => state.snackbar.queue[0]);


    useEffect(() => {
        return subscribeToSnackbars(( snackbar ) => dispatch(enqueueSnackbar(snackbar)));
    }, [ dispatch ]);


    return snackbar ? (
        <Snackbar
            key={snackbar.id}
            message={snackbar.message}
            severity={snackbar.severity}
            onClose={() => dispatch(dismissSnackbar(snackbar.id))}
        />
    ) : null;
};


export default SnackbarHost;
