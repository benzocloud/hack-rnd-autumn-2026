import assert from "node:assert/strict";

const origin = new URL(process.argv[2] || "http://127.0.0.1:3005");
const response = await fetch(new URL("/landing", origin), { signal: AbortSignal.timeout(15000) });
assert.equal(response.status, 200);
const html = await response.text();
assert.match(html, /Saldo Analytics/);
assert.doesNotMatch(html, /(?:src|href)="\/(?:_next|icons|favicons|fonts)\//);
const paths = new Set([
    "/landing/fonts/inter/Inter-Regular.woff2",
    "/landing/icons/saldo/saldo-analytics-black.svg",
    "/landing/icons/app/arrow-right.svg",
    "/landing/favicons/favicon.svg",
    ...Array.from(html.matchAll(/(?:src|href)="(\/landing\/[^"?#]+)(?:\?[^"#]*)?"/g), (match) => match[1])
        .filter((path) => /\.(?:js|css|svg|png|ico|woff2)$/.test(path)),
]);
assert.ok(Array.from(paths).some((path) => path.endsWith(".js")), "missing prefixed JavaScript");
assert.ok(Array.from(paths).some((path) => path.endsWith(".css")), "missing prefixed styles");
for (const path of paths) {
    const asset = await fetch(new URL(path, origin), { signal: AbortSignal.timeout(15000) });
    assert.equal(asset.status, 200, path);
    const type = asset.headers.get("content-type") || "";
    assert.doesNotMatch(type, /text\/html/, `asset returned HTML: ${path}`);
    if (path.endsWith(".css")) {
        assert.doesNotMatch(await asset.text(), /url\(["']?\/(?:fonts|icons)\//);
    } else {
        await asset.arrayBuffer();
    }
}
console.log(`PASS /landing and ${paths.size} prefixed assets`);
