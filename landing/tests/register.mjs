import { readFileSync, existsSync } from "node:fs";
import { registerHooks, createRequire } from "node:module";
import { resolve, dirname } from "node:path";
import { pathToFileURL, fileURLToPath } from "node:url";


const root = process.cwd();
const require = createRequire(resolve(root, "package.json"));
const ts = require("typescript");


registerHooks({
    resolve: ( specifier, context, nextResolve ) => {
        if (specifier === "axios") {
            return { url: pathToFileURL(resolve(root, "node_modules/axios/index.js")).href, shortCircuit: true };
        }

        let target;

        if (specifier.startsWith("@/")) {
            target = resolve(root, "src", specifier.slice(2));
        } else if (specifier.startsWith(".") && context.parentURL?.endsWith(".ts")) {
            target = resolve(dirname(fileURLToPath(context.parentURL)), specifier);
        }

        if (target) {
            const match = [ target, `${target}.ts`, `${target}/index.ts` ].find(( candidate ) => existsSync(candidate) && /\.(ts|json)$/.test(candidate));

            if (match) {
                return { url: pathToFileURL(match).href, shortCircuit: true };
            }
        }

        return nextResolve(specifier, context);
    },
    load: ( url, context, nextLoad ) => {
        if (url.startsWith(pathToFileURL(root).href) && url.endsWith(".ts")) {
            return { format: "module", source: ts.transpileModule(readFileSync(fileURLToPath(url), "utf8"), { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText, shortCircuit: true };
        }

        if (url.startsWith(pathToFileURL(resolve(root, "src")).href) && url.endsWith(".json")) {
            return { format: "module", source: `export default ${readFileSync(fileURLToPath(url), "utf8")}`, shortCircuit: true };
        }

        return nextLoad(url, context);
    },
});
