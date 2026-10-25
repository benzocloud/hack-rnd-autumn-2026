import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import ts from "typescript";


// Exercise component callbacks with isolated hooks; no browser renderer is installed.
const setup = () => {
    const source = readFileSync(new URL("../src/features/Imports/Imports.tsx", import.meta.url), "utf8").replace(/^import .*;$/gm, "").replace(/^export default Imports;$/m, "");
    const requests = [];
    const pages = {
        first: { items: Array.from({ length: 100 }, (_, index) => ({ row_number: index + 1, severity: "warning", message: `First ${index}` })), next_cursor: "second" },
        second: { items: [ { row_number: 101, severity: "warning", message: "Second page" } ], next_cursor: null },
    };
    let state;
    let index;
    let page = pages.first;
    const dependencies = {
        createElement: (type, props, ...children) => ({ type, props: { ...props, children } }),
        Fragment: "fragment",
        useState: (initial) => {
            const slot = index++;
            if (!(slot in state)) { state[slot] = initial; }
            return [ state[slot], (value) => { state[slot] = typeof value === "function" ? value(state[slot]) : value; } ];
        },
        useCallback: (callback) => callback,
        useCompany: () => ({ company: { id: "company" }, reload() {} }),
        useJobResource: () => ({ data: { status: "preview_ready", preview: { blocking_issue_count: 0, warning_count: 10001, balance_observations: [], can_commit: true, preview_version: "1" } }, loading: false, error: null }),
        useRequest: () => ({ busy: false }),
        useResource: (loader) => { loader(new AbortController().signal); return { data: page, loading: false, error: null, reload() {} }; },
        importsService: { listImportIssues: (request) => { requests.push(request); page = pages[request.cursor || "first"]; return Promise.resolve(page); } },
        styles: {},
        getErrorDetailMessage: (value) => value.message,
        formatDate: (value) => value,
        formatMoney: (value) => value,
        ...Object.fromEntries([ "Panel", "DataState", "JobState", "Notice", "Badge", "ActionButton", "FormError" ].map((name) => [ name, name ])),
    };
    const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.React, jsxFactory: "createElement", jsxFragmentFactory: "Fragment" } }).outputText;
    const components = new Function(...Object.keys(dependencies), `${compiled}\nreturn { ImportJob, ImportIssues, companiesInFile };`)(...Object.values(dependencies));
    const render = (component, props, slots) => { state = slots; index = 0; return component(props); };
    return { ...components, render, requests };
};


const nodes = (tree) => !tree || typeof tree !== "object" ? [] : Array.isArray(tree) ? tree.flatMap(nodes) : [ tree, ...nodes(tree.props?.children) ];
const text = (tree) => Array.isArray(tree) ? tree.map(text).join("") : tree && typeof tree === "object" ? text(tree.props?.children) : tree === false || tree == null ? "" : String(tree);
const button = (tree, label) => nodes(tree).find((node) => node.type === "ActionButton" && text(node) === label);


test("import issues load on expansion and replace one bounded page at a time", () => {
    const { ImportJob, ImportIssues, render, requests } = setup();
    const jobState = [];
    const job = () => render(ImportJob, { id: "import", onChanged() {} }, jobState);
    let tree = job();
    assert.equal(requests.length, 0);
    assert.equal(nodes(tree).some((node) => node.type === ImportIssues), false);
    assert.match(text(tree), /Посмотреть замечания \(10001\)/);

    nodes(tree).find((node) => node.type === "details").props.onToggle({ currentTarget: { open: true } });
    const issues = nodes(job()).find((node) => node.type === ImportIssues);
    const issueState = [];
    const issuePage = () => render(issues.type, issues.props, issueState);
    tree = issuePage();
    assert.deepEqual(requests, [ { company_id: "company", import_id: "import", limit: 100, cursor: undefined } ]);
    assert.equal(nodes(tree).filter((node) => node.type === "Badge").length, 100);
    assert.match(text(tree), /Всего замечаний: 10001/);
    assert.equal(button(tree, "Назад").props.disabled, true);

    button(tree, "Далее").props.onClick();
    tree = issuePage();
    assert.equal(requests.length, 2);
    assert.equal(requests[1].cursor, "second");
    assert.equal(nodes(tree).filter((node) => node.type === "Badge").length, 1);
    assert.doesNotMatch(text(tree), /First 0/);
    assert.equal(button(tree, "Далее").props.disabled, true);

    button(tree, "Назад").props.onClick();
    tree = issuePage();
    assert.equal(requests.at(-1).cursor, undefined);
    assert.equal(nodes(tree).filter((node) => node.type === "Badge").length, 100);
    nodes(job()).find((node) => node.type === "details").props.onToggle({ currentTarget: { open: false } });
    assert.equal(nodes(job()).some((node) => node.type === ImportIssues), false);
    assert.equal(requests.length, 3);
});


test("company codes are read from the file, including a semicolon inside quotes", async () => {
    const { companiesInFile } = setup();
    const file = new File([
        "\uFEFFcompany_id;operation_date;payment_purpose\n",
        "C001;2024-10-01;\"Оплата; НДС\"\n",
        "C001;2026-09-29;Комиссия\n",
        "C002;2025-01-01;Другая\n",
    ], "statements.csv", { type: "text/csv" });

    assert.deepEqual(await companiesInFile(file), [
        { id: "C001", rows: 2, from: "2024-10-01", to: "2026-09-29" },
        { id: "C002", rows: 1, from: "2025-01-01", to: "2025-01-01" },
    ]);
    await assert.rejects(companiesInFile(new File([ "amount;direction\n1;in\n" ], "other.csv", { type: "text/csv" })), /dataset_v1/);
});


test("company selection respects multiline quoted purposes and escaped quotes", async () => {
    const { companiesInFile } = setup();
    const file = new File([ 'company_id;operation_date;payment_purpose\r\nC001;2026-09-01;"First\r\nC_FAKE;2026-09-02;""quoted"" text"\r\nC002;2026-09-03;Other\r\n' ], "multiline.csv");
    assert.deepEqual(await companiesInFile(file), [
        { id: "C001", rows: 1, from: "2026-09-01", to: "2026-09-01" },
        { id: "C002", rows: 1, from: "2026-09-03", to: "2026-09-03" },
    ]);
    await assert.rejects(companiesInFile(new File(['company_id;operation_date;payment_purpose\nC001;2026-09-01;"unfinished'], "broken.csv")), /не закрыта кавычка/);
});
