import assert from "node:assert/strict";
import test from "node:test";

import { isAuthRoute, isPublicRoute, redirectToLogin } from "../src/helpers/auth.ts";


test("auth: public documents, auth routes and one redirect without loops", () => {
    const calls = [];

    global.window = { location: { pathname: "/login/", replace: ( path ) => calls.push(path) } };

    for (const path of [ "/login/", "/register", "/verify-code", "/forgot-password", "/reset-password", "/privacy-policy/", "/personal-data-consent" ]) {
        assert.ok(isPublicRoute(path));
        window.location.pathname = path;
        redirectToLogin();
    }

    assert.equal(calls.length, 0);
    assert.ok(isAuthRoute("/verify-code/"));
    assert.ok(!isAuthRoute("/privacy-policy"));
    assert.ok(!isPublicRoute("/account"));
    window.location.pathname = "/account";
    redirectToLogin();
    redirectToLogin();
    assert.deepEqual(calls, [ "/login" ]);
});
