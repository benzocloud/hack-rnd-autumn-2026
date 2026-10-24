import { spawnSync } from "node:child_process";
import { writeFileSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";


const require = createRequire(import.meta.url);
const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "/hack-rnd-autumn-2026";


if (basePath && !/^\/[a-zA-Z0-9_/-]+$/.test(basePath)) {
    throw new Error("NEXT_PUBLIC_BASE_PATH должен быть URL-путём без завершающего слеша.");
}

if (basePath.endsWith("/")) {
    throw new Error("Уберите завершающий слеш из NEXT_PUBLIC_BASE_PATH.");
}


const build = spawnSync(process.execPath, [ require.resolve("next/dist/bin/next"), "build", "--webpack" ], {
    stdio: "inherit",
    env: { ...process.env, PAGES_BUILD: "true", NEXT_PUBLIC_BASE_PATH: basePath, developerMode: "mock", NEXT_TELEMETRY_DISABLED: "1" },
});


if (build.status !== 0) {
    process.exit(build.status || 1);
}


const manifest = JSON.parse(readFileSync("out/favicons/site.webmanifest", "utf8"));

manifest.start_url = `${basePath}/`;
manifest.scope = `${basePath}/`;
writeFileSync("out/favicons/site.webmanifest", `${JSON.stringify(manifest, null, 4)}\n`);
writeFileSync("out/.nojekyll", "");

console.log(`Готово: out/. Режим mock, адрес приложения: ${basePath || "/"}.`);
