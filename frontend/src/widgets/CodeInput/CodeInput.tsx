"use client";

import { useRef, useState, type ClipboardEvent, type CSSProperties, type KeyboardEvent } from "react";

import { MOCK_CODE_LENGTH } from "@/helpers/verificationCode";

import styles from "./style.module.scss";


interface IProps {
    name?: string;
}


const CODE_LENGTH = MOCK_CODE_LENGTH;


const CodeInput = ( { name = "token" }: IProps ) => {
    const [ digits, setDigits ] = useState<string[]>(Array.from({ length: CODE_LENGTH }, () => ""));

    const inputs = useRef<Array<HTMLInputElement | null>>([]);


    const insertDigits = ( index: number, value: string ) => {
        const characters = value.replace(/\D/g, "");

        if (!characters) {
            if (!value) {
                setDigits(( current ) => current.map(( digit, position ) => position === index ? "" : digit));
            }

            return;
        }

        const start = characters.length >= CODE_LENGTH ? 0 : index;
        const inserted = Array.from(characters).slice(0, CODE_LENGTH - start);

        setDigits(( current ) => {
            const next = [ ...current ];

            inserted.forEach(( digit, offset ) => {
                next[start + offset] = digit;
            });

            return next;
        });

        inputs.current[Math.min(start + inserted.length, CODE_LENGTH - 1)]?.focus();
    };


    const pasteDigits = ( event: ClipboardEvent<HTMLInputElement>, index: number ) => {
        event.preventDefault();
        insertDigits(index, event.clipboardData.getData("text"));
    };


    const handleKey = ( event: KeyboardEvent<HTMLInputElement>, index: number ) => {
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
            event.preventDefault();
            inputs.current[index + (event.key === "ArrowLeft" ? -1 : 1)]?.focus();
        } else if (event.key === "Backspace" && !digits[index] && index > 0) {
            event.preventDefault();
            setDigits(( current ) => current.map(( digit, position ) => position === index - 1 ? "" : digit));
            inputs.current[index - 1]?.focus();
        }
    };


    return (
        <div className={styles.group} style={{ "--code-length": CODE_LENGTH } as CSSProperties} role="group" aria-label="Код подтверждения">
            <input type="hidden" name={name} value={digits.join("")} />

            { digits.map(( digit, index ) => (
                <div className={styles.cell} key={index}>
                    <input
                        ref={( input ) => { inputs.current[index] = input; }}
                        className={styles.input}
                        type="text"
                        inputMode="numeric"
                        autoComplete={index === 0 ? "one-time-code" : "off"}
                        aria-label={`Цифра ${index + 1} из ${CODE_LENGTH}`}
                        placeholder="0"
                        value={digit}
                        onChange={( event ) => insertDigits(index, event.target.value)}
                        onFocus={( event ) => event.currentTarget.select()}
                        onPaste={( event ) => pasteDigits(event, index)}
                        onKeyDown={( event ) => handleKey(event, index)}
                    />

                    { digit && <span className={styles.digit} aria-hidden="true" key={digit}>{ digit }</span> }
                </div>
            )) }
        </div>
    );
};


export default CodeInput;
