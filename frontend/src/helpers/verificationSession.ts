import type { IEmailVerificationChallenge } from "@/interfaces";


const storageName = "saldo-verification-session";


type VerificationSession = {
    email: string;
    challenge: IEmailVerificationChallenge | null;
};


type SessionStore = Pick<Storage, "getItem" | "setItem" | "removeItem">;


const emptySession = (): VerificationSession => ({ email: "", challenge: null });


const isChallenge = ( value: unknown ): value is IEmailVerificationChallenge => {
    if (typeof value !== "object" || value === null) {
        return false;
    }

    const challenge = value as Partial<IEmailVerificationChallenge>;

    return typeof challenge.challenge_id === "string" && challenge.challenge_id.length > 0
        && typeof challenge.expires_at === "string" && Number.isFinite(Date.parse(challenge.expires_at))
        && typeof challenge.message === "string";
};


const readVerificationSessionFrom = ( store: SessionStore ): VerificationSession => {
    try {
        const raw = store.getItem(storageName);

        if (!raw) {
            return emptySession();
        }

        const parsed = JSON.parse(raw) as Partial<VerificationSession>;
        const email = typeof parsed.email === "string" ? parsed.email : "";
        const challenge = isChallenge(parsed.challenge) && Date.parse(parsed.challenge.expires_at) > Date.now()
            ? parsed.challenge
            : null;

        return { email, challenge };
    } catch {
        return emptySession();
    }
};


const writeVerificationSessionTo = ( store: SessionStore, session: VerificationSession ) => {
    try {
        if (!session.email && !session.challenge) {
            store.removeItem(storageName);
            return;
        }

        store.setItem(storageName, JSON.stringify({
            email: session.email,
            challenge: session.challenge,
        }));
    } catch {
        // Private browsers can refuse storage; in-memory Redux still works for the same page.
    }
};


const browserStore = (): SessionStore | null => {
    try {
        if (typeof sessionStorage === "undefined") {
            return null;
        }

        return sessionStorage;
    } catch {
        return null;
    }
};


const readVerificationSession = (): VerificationSession => {
    const store = browserStore();

    return store ? readVerificationSessionFrom(store) : emptySession();
};


const writeVerificationSession = ( session: VerificationSession ) => {
    const store = browserStore();

    if (store) {
        writeVerificationSessionTo(store, session);
    }
};


export { readVerificationSessionFrom, writeVerificationSessionTo, readVerificationSession, writeVerificationSession, storageName };
export type { VerificationSession };
