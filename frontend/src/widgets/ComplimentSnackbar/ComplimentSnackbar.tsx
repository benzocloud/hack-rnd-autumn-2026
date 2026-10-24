"use client";

import { useEffect } from "react";

import { showInfoSnackbar } from "@/helpers/snackbar";
import startComplimentTimer from "@/helpers/startComplimentTimer";


const ComplimentSnackbar = () => {
    useEffect(() => startComplimentTimer(showInfoSnackbar), []);

    return null;
};


export default ComplimentSnackbar;
