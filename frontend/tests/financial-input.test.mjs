import assert from "node:assert/strict";
import test from "node:test";
import { AxiosError } from "axios";
import { parseMoney, moneyInput } from "../src/helpers/finance.ts";
import getErrorMessage, { getErrorCode } from "../src/helpers/getErrorMessage.ts";

const failure = (code) => new AxiosError("request failed", "ERR_BAD_RESPONSE", undefined, undefined, { data: { error: { code, message: "Server message" } }, status: 409 });

test("manual money preserves signed int64 boundaries and kopecks above Number precision", () => {
    for (const minor of ["9007199254740993", "9223372036854775807", "-9223372036854775808", "0", "-1"]) {
        assert.equal(parseMoney(moneyInput(minor), true), minor);
    }
    assert.equal(parseMoney("90 071 992 547 409,93"), "9007199254740993");
    for (const value of ["92233720368547758,08", "-92233720368547758,09", "1.001", "NaN", "Infinity"]) {
        assert.throws(() => parseMoney(value, true));
    }
    assert.throws(() => parseMoney("0"));
    assert.throws(() => parseMoney("-1"));
});

test("financial conflicts explain distinct recovery without pretending a successful write", () => {
    const cases = {
        IMPORT_PREVIEW_STALE: /Загрузите файл заново/,
        TRANSACTION_DUPLICATE_SUSPECTED: /не создана/,
        BALANCE_OBSERVATION_CONFLICT: /не может его перезаписать/,
        BALANCE_AS_OF_UNAVAILABLE: /нет подтверждённого остатка/,
        HISTORY_WINDOW_TOO_SHORT: /90 календарных дней/,
        HISTORY_RECONCILIATION_FAILED: /не согласуются/,
        FACTUAL_SOURCE_CONFLICT: /другим источником/,
        SOURCE_BINDING_CONFLICT: /другая компания/,
        DEPENDENCY_UNAVAILABLE: /временно недоступен/,
        CSRF_INVALID: /токен отклонён/,
    };
    for (const [code, message] of Object.entries(cases)) {
        assert.equal(getErrorCode(failure(code)), code);
        assert.match(getErrorMessage(failure(code)), message);
    }
    assert.equal(getErrorMessage(failure("FUTURE_CODE")), "Server message");
    assert.equal(getErrorCode(new Error("offline")), null);
});
