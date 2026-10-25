import { createSlice, type PayloadAction } from "@reduxjs/toolkit";

import { readVerificationSession, writeVerificationSession } from "@/helpers/verificationSession";
import type { IAuthState, IUser } from "@/interfaces";


const stored = readVerificationSession();
const initialState: IAuthState = {
    user: null,
    status: "idle",
    verificationEmail: stored.email,
    verificationChallenge: stored.challenge,
};


const persistVerification = ( state: IAuthState ) => {
    writeVerificationSession({ email: state.verificationEmail, challenge: state.verificationChallenge });
};


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
            persistVerification(state);
        },
        setVerificationChallenge: ( state, action: PayloadAction<IAuthState["verificationChallenge"]> ) => {
            state.verificationChallenge = action.payload;
            persistVerification(state);
        },
    },
});


const { setAuthUser, clearAuthUser, setAuthStatus, setVerificationEmail, setVerificationChallenge } = authSlice.actions;


export { setAuthUser, clearAuthUser, setAuthStatus, setVerificationEmail, setVerificationChallenge };
export default authSlice.reducer;
