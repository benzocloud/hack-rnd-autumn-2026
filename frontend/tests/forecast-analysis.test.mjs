import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
    categorySourceLabel,
    categorySourceTone,
    directionLabel,
    hasForecastAnalysis,
    hypothesisScheduleLabel,
    weekendShiftLabel,
} from "../src/helpers/forecastAnalysis.ts";


const bodies = JSON.parse(readFileSync(new URL("../src/mock/bodies.json", import.meta.url), "utf8"));


test("forecast analysis labels stay honest about source and hypothesis schedule", () => {
    assert.equal(categorySourceLabel("manual"), "Вручную");
    assert.equal(categorySourceLabel("rules"), "Анализ");
    assert.equal(categorySourceLabel("unknown"), "Не определено");
    assert.equal(categorySourceTone("manual"), "success");
    assert.equal(categorySourceTone("rules"), "neutral");
    assert.equal(categorySourceTone("unknown"), "warning");
    assert.equal(directionLabel("inflow"), "Поступление");
    assert.equal(directionLabel("outflow"), "Списание");
    assert.equal(weekendShiftLabel("previous"), "на предыдущий рабочий день");
    assert.equal(hypothesisScheduleLabel({ day_of_month: 7, weekend_shift: "previous" }), "7-е число · на предыдущий рабочий день");
    assert.equal(hypothesisScheduleLabel({ month_end: true, weekend_shift: "none" }), "конец месяца · без сдвига");
});


test("null analysis means not analyzed, never unknown classification", () => {
    assert.equal(hasForecastAnalysis(null), false);
    assert.equal(hasForecastAnalysis(undefined), false);
    assert.equal(hasForecastAnalysis(bodies.ForecastSucceeded.result.analysis), true);
});


test("seeded forecast fixture carries analysis; newly typed result keeps nullable field", () => {
    const analysis = bodies.ForecastSucceeded.result.analysis;

    assert.equal(analysis.analysis_version, "text_categories_monthly_v1");
    assert.ok(Array.isArray(analysis.classified_operations));
    assert.ok(analysis.classified_operations.some(( item ) => item.category_source === "manual"));
    assert.ok(analysis.classified_operations.some(( item ) => item.category_source === "rules"));
    assert.ok(analysis.classified_operations.some(( item ) => item.category_source === "unknown"));
    assert.equal(analysis.recurring_hypotheses[0].status, "hypothesis");
    assert.equal(typeof analysis.recurring_hypotheses[0].expected_amount_minor, "string");
    assert.equal(Object.hasOwn(bodies.ForecastSucceeded.result, "analysis"), true);
});
