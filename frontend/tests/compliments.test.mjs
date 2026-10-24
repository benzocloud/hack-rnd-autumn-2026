import assert from "node:assert/strict";
import test from "node:test";

import startComplimentTimer from "../src/helpers/startComplimentTimer.ts";


test("compliments: first at three minutes, random interval bounds and cleanup", ( context ) => {
    context.mock.timers.enable({ apis: [ "setTimeout" ] });

    const random = context.mock.method(Math, "random", () => 0);
    const messages = [];
    const stop = startComplimentTimer(( message ) => messages.push(message));

    context.mock.timers.tick(179_999);
    assert.equal(messages.length, 0);
    context.mock.timers.tick(1);
    assert.deepEqual(messages, [ "Отлично выглядите!" ]);
    context.mock.timers.tick(299_999);
    assert.equal(messages.length, 1);
    random.mock.mockImplementation(() => 0.999999);
    context.mock.timers.tick(1);
    assert.equal(messages.length, 2);
    context.mock.timers.tick(7_199_992);
    assert.equal(messages.length, 2);
    context.mock.timers.tick(8);
    assert.equal(messages.length, 3);
    stop();
    context.mock.timers.tick(7_200_000);
    assert.equal(messages.length, 3);
});
