import assert from "node:assert/strict";
import test from "node:test";

import { makeDemoForecast } from "../src/helpers/demoForecast.ts";
import formatMoney from "../src/helpers/formatMoney.ts";


test("all demo horizons contain consecutive business dates and exact RUB amounts", () => {
    for (const period of [ 30, 60, 90 ]) {
        const points = makeDemoForecast(period);

        assert.equal(points.length, period);
        assert.equal(points[0].date, "2026-10-24");

        points.forEach(( point, index ) => {
            assert.match(point.balance.amount_minor, /^-?\d+$/);
            assert.equal(point.balance.currency, "RUB");

            if (index) {
                assert.equal(new Date(point.date) - new Date(points[index - 1].date), 86400000);
            }
        });
    }
});

test("the warning agrees with the plotted first deficit and minimum balance", () => {
    const points = makeDemoForecast(30);
    const firstGap = points.find(( point ) => BigInt(point.balance.amount_minor) < BigInt(0));

    assert.equal(firstGap.date, "2026-11-13");
    assert.equal(firstGap.balance.amount_minor, "-4200000");
    assert.equal(Math.min(...points.map(( point ) => Number(point.balance.amount_minor))), -4200000);
});

test("delayed receipt returns to baseline after the selected delay window", () => {
    const base = makeDemoForecast(30);
    const delayed = makeDemoForecast(30, "delay");

    delayed.forEach(( point, day ) => {
        const difference = BigInt(base[day].balance.amount_minor) - BigInt(point.balance.amount_minor);

        assert.equal(difference, day >= 12 && day < 23 ? BigInt(18000000) : BigInt(0));
    });
});

test("a one-off expense affects every following day without changing earlier balances", () => {
    const base = makeDemoForecast(30);
    const expense = makeDemoForecast(30, "expense");

    expense.forEach(( point, day ) => {
        assert.equal(BigInt(base[day].balance.amount_minor) - BigInt(point.balance.amount_minor), day >= 10 ? BigInt(25000000) : BigInt(0));
    });
});

test("scenario generation is deterministic and never mutates the baseline", () => {
    const baseline = makeDemoForecast(90);
    const snapshot = structuredClone(baseline);

    for (const scenario of [ "delay", "sales", "expense", "recurring" ]) {
        assert.deepEqual(makeDemoForecast(90, scenario), makeDemoForecast(90, scenario));
        assert.notDeepEqual(makeDemoForecast(90, scenario), baseline);
    }

    assert.deepEqual(baseline, snapshot);
    assert.deepEqual(makeDemoForecast(90), snapshot);
});

test("money formatting preserves signed kopecks and large integer amounts", () => {
    assert.equal(formatMoney("-4200001"), "−42 000,01 руб.");
    assert.equal(formatMoney("0"), "0 руб.");
    assert.equal(formatMoney("900719925474099101"), "9 007 199 254 740 991,01 руб.");
});
