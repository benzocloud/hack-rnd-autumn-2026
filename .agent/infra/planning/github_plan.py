#!/usr/bin/env python3
"""Trusted Issue planning orchestration. Model output never selects write paths."""

import base64
import json
import os
import re
import sys
import urllib.error
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "runner"))
from github_api import REPOSITORY, GitHub, event, invoke_json, pull_request_discussion
from plan_policy import (
    command,
    fingerprint,
    identity,
    is_plan_branch,
    render,
    validate_context,
    validate_report,
)

MARKER = re.compile(r"<!-- agent-plan:issue=(\d+) -->")


def selected(api, payload):
    mode = command(
        payload,
        os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
        os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
    )
    if not mode:
        return None
    if mode != "scope":
        actor = urllib.parse.quote(payload["comment"]["user"]["login"], safe="")
        if api.request(f"/collaborators/{actor}/permission").get("permission") not in (
            "admin",
            "maintain",
            "write",
        ):
            return None
        current = api.request(f"/issues/comments/{int(payload['comment']['id'])}")
        if (
            current["body"] != payload["comment"]["body"]
            or current["user"]["login"] != payload["comment"]["user"]["login"]
        ):
            raise ValueError("Command comment changed")
    number = int(payload["issue"]["number"])
    pr = None
    if mode == "discuss-plan":
        pr = api.request(f"/pulls/{number}")
        match = MARKER.search(pr.get("body") or "")
        if not match:
            raise ValueError("/discuss-plan requires a managed plan PR")
        number = int(match[1])
        _, path = identity(number)
        if (
            pr["state"] != "open"
            or pr["head"].get("repo", {}).get("full_name") != REPOSITORY
            or not is_plan_branch(pr["head"]["ref"], number)
        ):
            raise ValueError("Only open same-repository managed plan PRs are accepted")
        files = api.pages(f"/pulls/{pr['number']}/files")
        if len(files) != 1 or files[0]["filename"] != path or pr["changed_files"] != 1:
            raise ValueError("Plan PR contains implementation changes; edit it manually")
    issue = api.request(f"/issues/{number}")
    if "pull_request" in issue or issue["state"] != "open":
        raise ValueError("Expected an open Issue")
    return mode, issue, pr


def output(**values):
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as out:
        for key, value in values.items():
            out.write(f"{key}={value}\n")


def authorize():
    request = selected(GitHub(), event())
    output(requested=str(request is not None).lower())


def default_source(api):
    branch = api.request("")["default_branch"]
    sha = api.request("/commits/" + urllib.parse.quote(branch, safe=""))["sha"]
    return branch, sha


def managed_prs(api, number):
    identity(number)
    candidates = set()
    # Plan PRs include Refs #N; the Issue timeline retains closed PR references.
    for item in api.pages(f"/issues/{number}/timeline"):
        if item.get("event") != "cross-referenced":
            continue
        source = item.get("source", {}).get("issue", {})
        candidate = source.get("number")
        if (
            type(candidate) is int
            and candidate > 0
            and "pull_request" in source
            and source.get("url") == f"https://api.github.com/repos/{REPOSITORY}/issues/{candidate}"
        ):
            candidates.add(candidate)
    plans = []
    for candidate in sorted(candidates):
        pr = api.request(f"/pulls/{candidate}")
        if (
            f"<!-- agent-plan:issue={number} -->" in (pr.get("body") or "")
            and pr["head"].get("repo", {}).get("full_name") == REPOSITORY
            and is_plan_branch(pr["head"]["ref"], number)
        ):
            plans.append(pr)
    return plans


def prepare():
    api, payload = GitHub(), event()
    request = selected(api, payload)
    output(requested="false")
    if request is None:
        return
    mode, issue, pr = request
    _, path = identity(issue["number"])
    base_branch, sha = default_source(api)
    existing = None
    if mode == "create-plan" and managed_prs(api, issue["number"]):
        # Do not spend another model run or create duplicate branches/PRs.
        print("A plan PR already exists for this Issue; use /discuss-plan there.")
        return
    if pr:
        sha = pr["head"]["sha"]
        content = api.request(f"/contents/{path}?ref={sha}")
        if content.get("encoding") != "base64" or content.get("size", 0) > 60000:
            raise ValueError("Unsupported plan file")
        existing = base64.b64decode(content["content"], validate=False).decode()
    context = {
        "repository": REPOSITORY,
        "mode": mode,
        "issue_number": issue["number"],
        "issue": issue,
        "issue_fingerprint": fingerprint(issue),
        "source_sha": sha,
        "base_branch": base_branch,
        "branch": pr["head"]["ref"] if pr else None,
        "plan_path": path,
        "request_id": payload.get("comment", {}).get("id", issue["id"]),
        "request_body": payload.get("comment", {}).get("body", ""),
        "issue_comments": api.pages(f"/issues/{issue['number']}/comments"),
        "open_prs": [
            {
                "number": p["number"],
                "title": p["title"],
                "body": p.get("body"),
                "head": p["head"]["ref"],
            }
            for p in api.pages("/pulls?state=open")
        ],
        "pr_number": pr["number"] if pr else None,
        "existing_plan": existing,
        **(
            pull_request_discussion(api, pr["number"])
            if pr
            else {"pr_comments": [], "pr_reviews": [], "pr_review_comments": []}
        ),
    }
    validate_context(context)
    policies = {
        name: Path(name).read_text()
        for name in (
            "AGENTS.md",
            ".agent/instructions.md",
            ".agent/decisions.md",
            ".agent/workflows/issue-planning.md",
            ".agent/workflows/context-and-memory.md",
            ".agent/workflows/git-conventions.md",
            ".agent/references/go-services.md",
        )
    }
    archive = api.request(f"/tarball/{sha}", raw=True)
    Path("request.json").write_text(
        json.dumps(
            {
                "context": context,
                "policies": policies,
                "archive": base64.b64encode(archive).decode(),
            }
        )
    )
    Path("context.json").write_text(json.dumps(context))
    output(requested="true")


def invoke():
    context = json.loads(Path("context.json").read_text())
    validate_context(context)
    report = invoke_json("agent-" + context["mode"])
    validate_report(report, context)
    Path("plan-result.json").write_text(json.dumps({"context": context, "report": report}))


def write_plan_commit(api, parent, path, markdown, request_id):
    # Fixed caller-derived path, never a path supplied by the model.
    if not re.fullmatch(r"\.agent/plans/issue-[1-9][0-9]*-fix\.md", path):
        raise ValueError("Refusing a source implementation write")
    commit = api.request(f"/git/commits/{parent}")
    tree = api.request(
        "/git/trees",
        {
            "base_tree": commit["tree"]["sha"],
            "tree": [{"path": path, "mode": "100644", "type": "blob", "content": markdown}],
        },
    )
    return api.request(
        "/git/commits",
        {
            "message": f"chore: update Issue fix plan (request {request_id})",
            "tree": tree["sha"],
            "parents": [parent],
        },
    )["sha"]


def publish():
    api, payload = GitHub(), event()
    package = json.loads(Path("plan-result.json").read_text())
    context, report = package["context"], package["report"]
    validate_report(report, context)
    request = selected(api, payload)
    if request is None:
        raise ValueError("Request no longer authorized")
    mode, issue, pr = request
    request_id = payload.get("comment", {}).get("id", issue["id"])
    if (
        mode != context["mode"]
        or issue["number"] != context["issue_number"]
        or request_id != context["request_id"]
        or fingerprint(issue) != context["issue_fingerprint"]
        or payload.get("comment", {}).get("body", "") != context["request_body"]
    ):
        raise ValueError("Issue/request changed; discard stale plan")
    branch, path = identity(issue["number"], report["branch_description"])
    if (pr and pr["head"]["ref"] != context["branch"]) or path != context["plan_path"]:
        raise ValueError("Plan destination differs from trusted policy")
    if pr:
        branch = context["branch"]
    base_branch, current_sha = default_source(api)
    if (pr["head"]["sha"] if pr else current_sha) != context[
        "source_sha"
    ] or base_branch != context["base_branch"]:
        raise ValueError("Source changed; request a fresh plan")
    markdown = render(report, context)
    marker = f"<!-- agent-plan-request:{mode}:{request_id} -->"
    destination = pr["number"] if pr else issue["number"]
    comments = api.pages(f"/issues/{destination}/comments")
    if any(
        marker in (c.get("body") or "") and c.get("user", {}).get("type") == "Bot" for c in comments
    ):
        print("Already published this request")
        return
    if mode == "scope":
        api.request(
            f"/issues/{issue['number']}/comments",
            {
                "body": marker
                + "\n\n"
                + markdown
                + "\nДля ветки и draft PR с планом: `/create-plan`. Изменения плана обсуждаются в его PR командой `/discuss-plan <изменения>`."
            },
        )
        return
    if mode == "create-plan":
        existing = managed_prs(api, issue["number"])
        if existing:
            api.request(
                f"/issues/{issue['number']}/comments",
                {"body": marker + "\nПлан уже существует: " + existing[0]["html_url"]},
            )
            return
        try:
            api.request("/git/ref/heads/" + branch)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        else:
            raise ValueError(
                "Plan branch already exists without a managed PR; refusing to overwrite"
            )
        sha = write_plan_commit(api, current_sha, path, markdown, request_id)
        api.request("/git/refs", {"ref": "refs/heads/" + branch, "sha": sha})
        prefix = next(
            (p for p in ("INFRA/ML:", "ML:", "INFRA:") if issue["title"].startswith(p)), "INFRA:"
        )
        created = api.request(
            "/pulls",
            {
                "title": f"{prefix} chore: plan for Issue #{issue['number']}",
                "head": branch,
                "base": base_branch,
                "draft": True,
                "body": f"<!-- agent-plan:issue={issue['number']} -->\nRefs #{issue['number']}\n\n"
                "Только план исправления. Реализация отдельно после решения разработчика.\n\n"
                "Для изменения плана: `/discuss-plan <изменения>`. Ревью — только `/agent-review`.\n"
                "CI не подтверждён; создание PR через GITHUB_TOKEN само по себе не запускает другие workflow.",
            },
        )
        api.request(
            f"/issues/{issue['number']}/comments",
            {"body": marker + "\nСоздан draft PR с планом: " + created["html_url"]},
        )
    else:
        # New commit has the exact reviewed head as parent. force=false makes a
        # concurrent branch update fail rather than overwriting another edit.
        current = api.request("/git/commits/" + context["source_sha"])
        if f"request {request_id})" in current["message"]:
            print("This request already updated the plan commit")
        else:
            sha = write_plan_commit(api, context["source_sha"], path, markdown, request_id)
            api.request("/git/refs/heads/" + branch, {"sha": sha, "force": False}, method="PATCH")
        api.request(
            f"/issues/{pr['number']}/comments",
            {"body": marker + "\nПлан обновлён в `" + path + "`. Исходники не изменялись."},
        )


if __name__ == "__main__":
    {"authorize": authorize, "prepare": prepare, "invoke": invoke, "publish": publish}[
        sys.argv[1]
    ]()
