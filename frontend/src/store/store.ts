import { configureStore } from "@reduxjs/toolkit";

import authReducer from "./authSlice";
import companyReducer from "./companySlice";
import snackbarReducer from "./snackbarSlice";


const makeStore = () => {
    return configureStore({ reducer: { auth: authReducer, company: companyReducer, snackbar: snackbarReducer } });
};


type AppStore = ReturnType<typeof makeStore>;
type RootState = ReturnType<AppStore["getState"]>;
type AppDispatch = AppStore["dispatch"];


export type { AppStore, RootState, AppDispatch };
export default makeStore;
