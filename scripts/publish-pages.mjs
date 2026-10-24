import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";


const run = ( args, options = {} ) => {
    const result = spawnSync("git", args, { encoding: "utf8", ...options });

    if (result.status !== 0) {
        throw new Error(result.stderr || result.error?.message || `git ${args[0]} завершился с ошибкой.`);
    }

    return result.stdout.trim();
};


if (!existsSync("out/.nojekyll") || !existsSync("out/login/index.html")) {
    throw new Error("Сначала выполните npm run build:pages.");
}


const temporary = mkdtempSync(join(tmpdir(), "saldo-pages-"));


try {
    const remote = run([ "ls-remote", "--heads", "origin", "gh-pages" ]);

    if (remote) {
        run([ "fetch", "origin", "gh-pages" ]);
    }

    const parent = remote ? remote.split(/\s+/)[0] : "";
    const env = { ...process.env, GIT_INDEX_FILE: join(temporary, "index") };
    const output = resolve("out");

    run([ "read-tree", "--empty" ], { env });
    run([ `--work-tree=${output}`, "add", "--all", "--force", "." ], { env, cwd: output });

    const tree = run([ "write-tree" ], { env });
    const commit = run([ "commit-tree", tree, ...(parent ? [ "-p", parent ] : []), "-m", "build: publish GitHub Pages frontend" ]);

    run([ "update-ref", "refs/heads/gh-pages", commit ]);
    console.log(run([ "push", "origin", "gh-pages:gh-pages" ]));
    console.log(`Опубликована ветка gh-pages: ${commit}`);
} finally {
    rmSync(temporary, { recursive: true, force: true });
}
