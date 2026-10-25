"use client";

import { useState, type ReactNode } from "react";
import { Provider } from "react-redux";

import { SnackbarHost } from "@/widgets";

import makeStore from "./store";


interface IProps {
    children: ReactNode;
}


const StoreProvider = ( { children }: IProps ) => {
    const [ store ] = useState(makeStore);


    return (
        <Provider store={store}>
            { children }
            <SnackbarHost />
        </Provider>
    );
};


export default StoreProvider;
