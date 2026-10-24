import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";


test("auth redirects: optional base path, public routes and one redirect without loops", () => {
    for (const basePath of [ "", "/demo" ]) {
        const result = spawnSync(process.execPath, [ "--import", "./tests/register.mjs", "--input-type=module", "-e", `
            import assert from "node:assert/strict";
            import { withBasePath, withoutBasePath } from "./src/helpers/basePath.ts";
            import { isAuthRoute, isPublicRoute, redirectToLogin } from "./src/helpers/auth.ts";
            const prefix = process.env.NEXT_PUBLIC_BASE_PATH;
            assert.equal(withBasePath("/icons/logo.svg"), prefix + "/icons/logo.svg");
            assert.equal(withoutBasePath(prefix + "/account/"), "/account/");
            assert.ok(isAuthRoute(prefix + "/login/"));
            assert.ok(isAuthRoute("/verify-code"));
            assert.ok(isPublicRoute(prefix + "/privacy-policy/"));
            assert.ok(isPublicRoute(prefix + "/personal-data-consent/"));
            assert.ok(!isPublicRoute(prefix + "/account/"));
            const calls = [];
            global.window = { location: { pathname: prefix + "/login/", replace: (path) => calls.push(path) } };
            redirectToLogin();
            assert.equal(calls.length, 0);
            window.location.pathname = prefix + "/account/";
            redirectToLogin();
            redirectToLogin();
            assert.deepEqual(calls, [prefix + "/login"]);
        ` ], { env: { ...process.env, NEXT_PUBLIC_BASE_PATH: basePath }, encoding: "utf8" });

        assert.equal(result.status, 0, result.stderr || result.stdout);
    }
});
