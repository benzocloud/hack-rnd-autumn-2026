#!/usr/bin/env python3
"""Trusted GitHub-hosted orchestration; never execute scripts from the PR."""
import base64
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.parse
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "runner"))
from github_api import GitHub, event, invoke_json
from review_policy import REPOSITORY, is_request, skills_for, scope_conflict, validate_context, validate_report



def select_request(api, payload):
    if not is_request(payload, os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
                      os.environ.get("GITHUB_RUN_ATTEMPT", "1")):
        return None
    actor = payload["comment"]["user"]["login"]
    permission = api.request("/collaborators/" + urllib.parse.quote(actor, safe="") + "/permission")
    if permission.get("permission") not in ("admin", "maintain", "write"):
        print("Ignored /agent-review: requester lacks repository write permission")
        return None
    pr = api.request(f"/pulls/{int(payload['issue']['number'])}")
    if pr["state"] != "open" or pr["head"].get("repo", {}).get("full_name") != REPOSITORY:
        raise ValueError("Only open, same-repository PRs are enabled in this pilot")
    return pr


def prepare():
    api, payload = GitHub(), event()
    pr = select_request(api, payload)
    output = Path(os.environ["GITHUB_OUTPUT"])
    with output.open("a") as out:
        out.write("requested=false\n")
    if pr is None:
        return
    number, head, base = pr["number"], pr["head"]["sha"], pr["base"]["sha"]
    files = api.pages(f"/pulls/{number}/files")
    if len(files) != pr["changed_files"]:
        raise ValueError("Changed-file list is incomplete")
    names = [item["filename"] for item in files]
    scope, skills = skills_for(pr["title"], names)
    issue_pattern = r"(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?|refs?)\s+(?:#|https://github\.com/" + re.escape(REPOSITORY) + r"/issues/)(\d+)"
    issue_numbers = sorted({int(n) for n in re.findall(issue_pattern, pr.get("body") or "", re.I)})
    if not issue_numbers:
        raise ValueError("PR must link its task using Refs/Closes/Fixes/Resolves #NUMBER")
    if len(issue_numbers) > 10:
        raise ValueError("Too many linked tasks for one review")
    issues = []
    for n in issue_numbers:
        issue = api.request(f"/issues/{n}")
        if "pull_request" in issue:
            raise ValueError("A linked task must be an Issue, not another PR")
        issues.append({"number": n, "title": issue["title"], "body": issue.get("body"),
                       "comments": api.pages(f"/issues/{n}/comments")})
    context = {"repository": REPOSITORY, "pr_number": number, "comment_id": payload["comment"]["id"],
               "head_sha": head, "base_sha": base, "title": pr["title"], "body": pr.get("body"),
               "scope": scope, "required_skills": skills, "files": names, "issues": issues,
               "pr_comments": api.pages(f"/issues/{number}/comments"),
               "ci": {"checks": api.pages(f"/commits/{head}/check-runs", "check_runs"),
                      "status": api.request(f"/commits/{head}/status")}}
    validate_context(context)
    diff = api.request(f"/pulls/{number}", raw=True, accept="application/vnd.github.diff").decode()
    if len(diff.encode()) > 2 * 1024 * 1024:
        raise ValueError("Diff too large for this pilot")
    archive = api.request(f"/tarball/{head}", raw=True)
    current = api.request(f"/pulls/{number}")
    if current["head"]["sha"] != head or current["base"]["sha"] != base or current["title"] != pr["title"]:
        raise ValueError("PR changed while context was collected; request a fresh review")
    trusted = {name: Path(name).read_text() for name in (
        "AGENTS.md", ".agent/instructions.md", ".agent/decisions.md",
        ".agent/workflows/automated-review.md", ".agent/workflows/context-and-memory.md",
        ".agent/prompts/pr-review.md", ".agent/workflows/git-conventions.md",
        ".agent/references/go-services.md")}
    package = {"context": context, "archive": base64.b64encode(archive).decode(), "diff": diff, "policies": trusted}
    Path("request.json").write_text(json.dumps(package))
    Path("context.json").write_text(json.dumps(context))
    with output.open("a") as out:
        out.write("requested=true\n")


def authorize():
    # This job has no concurrency group: unauthorized comments cannot cancel a review.
    allowed = select_request(GitHub(), event()) is not None
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as out:
        out.write("requested=" + str(allowed).lower() + "\n")


def invoke():
    context = json.loads(Path("context.json").read_text())
    if scope_conflict(context):
        report = {"head_sha": context["head_sha"], "base_sha": context["base_sha"], "scope": context["scope"],
                  "status": "incomplete", "memory": "unavailable", "skills": [],
                  "checks": ["Trusted title/changed-file routing check; model was not invoked"],
                  "limitations": ["ML: conflicts with Go changes. Correct the title or split the PR, then submit a new /agent-review comment."],
                  "findings": []}
        validate_report(report, context)
        Path("report.json").write_text(json.dumps({"context": context, "report": report, "loaded_skills": []}))
        Path("request.json").unlink(missing_ok=True)
        return
    result = invoke_json("agent-review")
    validate_report(result["report"], context)
    receipts = result["loaded_skills"]
    if [v["name"] for v in receipts] != context["required_skills"] or any(
            not re.fullmatch(r"[0-9a-f]{64}", v["sha256"]) for v in receipts):
        raise ValueError("Missing pinned skill loading receipts")
    Path("report.json").write_text(json.dumps({"context": context, **result}))


def publish():
    api, payload = GitHub(), event()
    package = json.loads(Path("report.json").read_text())
    context, report = package["context"], package["report"]
    validate_report(report, context)
    if context["pr_number"] != payload["issue"]["number"] or context["comment_id"] != payload["comment"]["id"]:
        raise ValueError("Report does not belong to this comment request")
    pr = select_request(api, payload)
    if (pr is None or pr["head"]["sha"] != context["head_sha"]
            or pr["base"]["sha"] != context["base_sha"] or pr["title"] != context["title"]):
        print("Discarded stale review. A fresh /agent-review comment is required.")
        return
    for comment in reversed(api.pages(f"/issues/{pr['number']}/comments")):
        if (comment["id"] > context["comment_id"] and comment.get("body") == "/agent-review"
                and comment.get("user", {}).get("type") == "User"):
            login = urllib.parse.quote(comment["user"]["login"], safe="")
            if api.request("/collaborators/" + login + "/permission").get("permission") in ("admin", "maintain", "write"):
                print("Discarded result superseded by a newer authorized request")
                return
    marker = f"<!-- agent-review:{context['comment_id']}:{context['head_sha']} -->"
    if any(marker in (item.get("body") or "") for item in api.pages(f"/pulls/{pr['number']}/reviews")):
        print("This request/SHA was already published")
        return
    body = [marker, f"Agent review: **{report['status']}**", f"Commit: `{context['head_sha']}`",
            f"Scope: `{report['scope']}`", "GBrain: unavailable (deferred).",
            "Skills reported applied: " + ", ".join(report["skills"])]
    ci = context["ci"]
    body.append("GitHub CI metadata: combined status `" + str(ci.get("status", {}).get("state", "unknown"))
                + "`; check runs: " + str(len(ci.get("checks", []))) + ". No local tests were executed by this static reviewer.")
    for item in report["findings"]:
        link = f"https://github.com/{REPOSITORY}/blob/{context['head_sha']}/{urllib.parse.quote(item['file'])}#L{item['line']}"
        body += [f"\n**{item['severity']}** — [{item['file']}:{item['line']}]({link})",
                 item["evidence"], "Impact: " + item["impact"], "Fix: " + item["fix"]]
    if not report["findings"]:
        body.append("No actionable findings reported; this is not approval or proof that tests passed.")
    body += ["\nChecks actually run:"] + ["- " + v for v in report["checks"]]
    body += ["\nLimitations:"] + ["- " + v for v in report["limitations"]]
    rendered = "\n\n".join(body)
    if len(rendered) > 60000:
        raise ValueError("Review too large to publish safely")
    # COMMENT is advisory: never approve, request merge, or deploy.
    api.request(f"/pulls/{pr['number']}/reviews", {"commit_id": context["head_sha"], "body": rendered, "event": "COMMENT"})


if __name__ == "__main__":
    {"authorize": authorize, "prepare": prepare, "invoke": invoke, "publish": publish}[sys.argv[1]]()
