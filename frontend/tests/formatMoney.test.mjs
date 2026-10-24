import assert from "node:assert/strict";
import test from "node:test";

import formatMoney from "../src/helpers/formatMoney.ts";


test("money labels preserve kopecks, negatives and values above Number precision", () => {
    assert.equal(formatMoney("1"), "0,01 руб.");
    assert.equal(formatMoney("-101"), "−1,01 руб.");
    assert.equal(formatMoney("123456789012345678"), "1 234 567 890 123 456,78 руб.");
});
