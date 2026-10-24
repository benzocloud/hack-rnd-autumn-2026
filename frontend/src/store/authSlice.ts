import { createSlice, type PayloadAction } from "@reduxjs/toolkit";

import type { IAuthState, IUser } from "@/interfaces";


const initialState: IAuthState = { user: null, status: "idle", verificationEmail: "" };


const authSlice = createSlice({
    name: "auth",
    initialState,
    reducers: {
        setAuthUser: ( state, action: PayloadAction<IUser> ) => {
            state.user = action.payload;
            state.status = "ready";
        },
        clearAuthUser: ( state ) => {
            state.user = null;
            state.status = "idle";
        },
        setAuthStatus: ( state, action: PayloadAction<IAuthState["status"]> ) => {
            state.status = action.payload;
        },
        setVerificationEmail: ( state, action: PayloadAction<string> ) => {
            state.verificationEmail = action.payload;
        },
    },
});


const { setAuthUser, clearAuthUser, setAuthStatus, setVerificationEmail } = authSlice.actions;


export { setAuthUser, clearAuthUser, setAuthStatus, setVerificationEmail };
export default authSlice.reducer;
