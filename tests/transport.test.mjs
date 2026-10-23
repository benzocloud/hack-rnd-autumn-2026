import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";


test("all modes except exact mock preserve regular API transport and CSRF", () => {
    for (const mode of [ "dev", "test", "MOCK", "custom", undefined ]) {
        const env = { ...process.env };

        if (mode === undefined) {
            delete env.developerMode;
        } else {
            env.developerMode = mode;
        }

        const result = spawnSync(process.execPath, [ "--import", "./tests/register.mjs", "tests/check-transport.mjs" ], { env, encoding: "utf8" });

        assert.equal(result.status, 0, `${mode}: ${result.stderr}`);
    }
});
