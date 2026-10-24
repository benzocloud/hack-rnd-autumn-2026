import { createSlice, type PayloadAction } from "@reduxjs/toolkit";

import type { ICompany } from "@/interfaces";


const initialState: { selected: ICompany | null } = { selected: null };


const companySlice = createSlice({
    name: "company",
    initialState,
    reducers: {
        setSelectedCompany: ( state, action: PayloadAction<ICompany | null> ) => {
            state.selected = action.payload;
        },
    },
});


const { setSelectedCompany } = companySlice.actions;


export { setSelectedCompany };
export default companySlice.reducer;
