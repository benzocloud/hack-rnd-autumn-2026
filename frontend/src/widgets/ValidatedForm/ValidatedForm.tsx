"use client";

import { createContext, useContext, useEffect, useRef, useState, type FormHTMLAttributes } from "react";

import { validateTextFields, type FieldErrors } from "@/helpers/fieldErrors";
import { showErrorSnackbar } from "@/helpers/snackbar";


interface IValidationRequest {
    fieldErrors: FieldErrors;
    clearFieldError: ( name?: string ) => void;
}

interface IProps extends FormHTMLAttributes<HTMLFormElement> {
    request?: IValidationRequest;
    validate?: ( form: FormData ) => FieldErrors;
}


const FieldErrorContext = createContext<FieldErrors>({});
const ClearFieldErrorContext = createContext<( name: string ) => void>(() => {});
const useFieldError = ( name?: string ) => useContext(FieldErrorContext)[name || ""];
const useClearFieldError = () => useContext(ClearFieldErrorContext);


const focusField = ( form: HTMLFormElement | null, name: string ) => {
    const field = form?.querySelector<HTMLElement>(`[name="${CSS.escape(name)}"]:not([type="hidden"]), [data-validation-name="${CSS.escape(name)}"]`);
    (field?.querySelector<HTMLElement>('[role="combobox"]') || field)?.focus();
};


const ValidatedForm = ( { request, validate, onSubmit, onChange, children, ...props }: IProps ) => {
    const [ errors, setErrors ] = useState<FieldErrors>({});
    const formRef = useRef<HTMLFormElement>(null);
    const fieldErrors = { ...request?.fieldErrors, ...errors };
    const clearFieldError = ( name: string ) => {
        setErrors(( current ) => Object.fromEntries(Object.entries(current).filter(( [ field ] ) => field !== name)));
        request?.clearFieldError(name);
    };

    useEffect(() => {
        const name = Object.keys(request?.fieldErrors || {})[0];
        if (name) {
            focusField(formRef.current, name);
        }
    }, [ request?.fieldErrors ]);


    return (
        <FieldErrorContext value={fieldErrors}>
            <ClearFieldErrorContext value={clearFieldError}>
            <form {...props} ref={formRef} noValidate onChange={( event ) => {
                const target = event.target;
                if (!(target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement)) { return; }
                const name = target.dataset.validationName || target.name;
                clearFieldError(name);
                onChange?.(event);
            }} onSubmit={( event ) => {
                event.preventDefault();
                const form = event.currentTarget;
                request?.clearFieldError();
                const next = validate ? validate(new FormData(form)) : validateTextFields(Array.from(form.elements).flatMap(( element ) => {
                    if (!(element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) || !element.name || element.disabled || (element instanceof HTMLInputElement && [ "hidden", "checkbox", "radio", "file", "submit" ].includes(element.type))) { return []; }

                    return [{ name: element.name, label: element.labels?.[0]?.textContent || element.name, value: element.value, required: element.required, maximum: element.dataset.maximum ? Number(element.dataset.maximum) : undefined, type: element.type, money: element.name === "amount", allowNegative: element.dataset.allowNegative === "true" }];
                }));
                setErrors(next);

                if (Object.keys(next).length) {
                    showErrorSnackbar(Object.values(next)[0]);
                    const name = Object.keys(next)[0];
                    focusField(form, name);
                    return;
                }

                void onSubmit?.(event);
            }}>
                { children }
            </form>
            </ClearFieldErrorContext>
        </FieldErrorContext>
    );
};


export { useFieldError, useClearFieldError };
export default ValidatedForm;
