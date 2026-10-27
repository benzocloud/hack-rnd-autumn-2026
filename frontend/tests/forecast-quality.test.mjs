import assert from "node:assert/strict";
import test from "node:test";

import {
    forecastQualityDescription,
    isDashboardHiddenLimitation,
    visibleDashboardLimitations,
} from "../src/helpers/forecastQuality.ts";


test("dashboard hides the three noisy limitation messages", () => {
    const hidden = [
        "historical_availability_unverified",
        "history_reconstructed_from_covered_facts_and_cutoff_eod",
        "not_calibrated",
        "Не подтверждено, когда сведения об операциях стали доступны.",
        "Вероятность кассового разрыва не откалибрована и не оценивается.",
    ];

    for (const value of hidden) {
        assert.equal(isDashboardHiddenLimitation(value), true, value);
    }

    assert.equal(
        forecastQualityDescription("historical_availability_unverified"),
        "Не подтверждено, когда сведения об операциях стали доступны.",
    );

    const visible = visibleDashboardLimitations([
        "historical_availability_unverified",
        "csv_history_completeness_unverified",
        "history_reconstructed_from_covered_facts_and_cutoff_eod",
        "not_calibrated",
        "historical_calendar_unavailable",
    ]);

    assert.deepEqual(visible, [
        "csv_history_completeness_unverified",
        "historical_calendar_unavailable",
    ]);
});
