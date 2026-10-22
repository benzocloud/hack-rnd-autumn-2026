import json
import base64
import io
import os
from pathlib import Path
import sys
import tempfile
import unittest
import urllib.error
from unittest.mock import patch
from contextlib import nullcontext, redirect_stdout

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
from plan_policy import command, identity, fingerprint, validate_report, render, is_plan_branch
import github_plan
import ssh_plan
from github_api import GitHub


def issue():
    return {"number": 7, "id": 70, "title": "ML: fix: empty prediction", "body": "Return an empty result", "state": "open"}


def context():
    i = issue()
    return {"repository": github_plan.REPOSITORY, "mode": "scope", "source_sha": "a" * 40,
            "issue_number": 7, "issue": i, "issue_fingerprint": fingerprint(i),
            "request_id": 70, "request_body": "", "branch": identity(7)[0], "plan_path": identity(7)[1], "base_branch": "main"}


def report():
    return {"source_sha": "a" * 40, "issue_number": 7, "status": "ready", "summary": "Handle an empty prediction", "branch_description": "handle_empty_prediction",
            "in_scope": ["Empty input"], "out_of_scope": ["Training"], "affected_files": ["ml/predict.py"],
            "evidence": ["ml/predict.py:2 indexes the input"], "steps": ["Add an empty-input guard"],
            "acceptance": ["Empty input returns empty result"], "risks": [], "questions": []}


class PlanningTests(unittest.TestCase):
    def test_meaningful_branch_description_is_safe_and_issue_bound(self):
        branch, path = identity(123, "handle_empty_input")
        self.assertEqual(branch, "runner/fix/handle_empty_input/issue_123")
        self.assertEqual(path, ".agent/plans/issue-123-fix.md")
        self.assertTrue(is_plan_branch(branch, 123))
        self.assertFalse(is_plan_branch(branch, 124))
        self.assertFalse(is_plan_branch("runner/feat/handle_empty_input/issue_123", 123))
        for description in ("../main", "Fix Input", "with/slash", "a" * 49, "empty__input", ""):
            with self.subTest(description=description), self.assertRaises(ValueError):
                identity(123, description)

    def test_plan_discussion_cannot_rename_branch(self):
        c = context(); c.update(mode="discuss-plan", existing_plan="Plan", branch=identity(7, "handle_empty_prediction")[0])
        r = report(); r["branch_description"] = "change_another_thing"
        with self.assertRaisesRegex(ValueError, "existing branch"):
            validate_report(r, c)

    def test_legacy_plan_accepts_discussion_without_renaming(self):
        c = context(); c.update(mode="discuss-plan", existing_plan="Plan", branch="codex/issue-7-plan")
        self.assertTrue(is_plan_branch(c["branch"], 7))
        self.assertFalse(is_plan_branch(c["branch"], 8))
        validate_report(report(), c)
        payload = {"action": "created", "issue": {"number": 10, "pull_request": {}},
                   "comment": {"id": 3, "body": "/discuss-plan refine", "user": {"type": "User", "login": "qa"}}}
        pr = {"number": 10, "state": "open", "body": "<!-- agent-plan:issue=7 -->",
              "head": {"ref": c["branch"], "repo": {"full_name": github_plan.REPOSITORY}}, "changed_files": 1}
        class API:
            def request(self, path):
                return {"/collaborators/qa/permission": {"permission": "write"},
                        "/issues/comments/3": payload["comment"], "/pulls/10": pr, "/issues/7": issue()}[path]
            def pages(self, path):
                assert path == "/pulls/10/files"
                return [{"filename": c["plan_path"]}]
        self.assertEqual(github_plan.selected(API(), payload), ("discuss-plan", issue(), pr))
        with patch.object(ssh_plan.os, "geteuid", return_value=1000), patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": "agent-discuss-plan"}), patch.object(ssh_plan.sys, "stdin") as stdin, patch.object(ssh_plan, "workspace", return_value=nullcontext(Path("unused"))), patch.object(ssh_plan, "infer", return_value=report()) as infer, redirect_stdout(io.StringIO()):
            stdin.buffer = io.BytesIO(json.dumps({"context": c}).encode())
            ssh_plan.main()
        self.assertNotIn("enum", infer.call_args.args[1]["properties"]["branch_description"])

    def test_managed_plans_use_paginated_issue_links_not_repository_history(self):
        def reference(number, repository=github_plan.REPOSITORY):
            return {"event": "cross-referenced", "source": {"issue": {
                "number": number, "url": f"https://api.github.com/repos/{repository}/issues/{number}", "pull_request": {}}}}
        class API(GitHub):
            def __init__(self):
                self.calls = []
            def request(self, path):
                self.calls.append(path)
                if path == "/issues/7/timeline?per_page=100&page=1":
                    return [{"event": "commented"}] * 99 + [reference(10)]
                if path == "/issues/7/timeline?per_page=100&page=2":
                    return [reference(10), reference(11), reference(12), reference(13), reference(14, "other/repo")]
                number = int(path.removeprefix("/pulls/"))
                return {"number": number, "state": "closed", "body": "<!-- agent-plan:issue=7 -->" if number != 12 else "Other PR",
                        "head": {"ref": "codex/issue-7-plan" if number == 10 else identity(7, "handle_empty_prediction")[0],
                                 "repo": {"full_name": github_plan.REPOSITORY if number != 13 else "other/repo"}}}
        api = API()
        self.assertEqual([p["number"] for p in github_plan.managed_prs(api, 7)], [10, 11])
        self.assertEqual(api.calls.count("/pulls/10"), 1)
        self.assertNotIn("/pulls/14", api.calls)

    def test_all_new_issues_are_assessed_without_qa_filter(self):
        self.assertEqual(command({"action": "opened", "issue": issue()}, "issues"), "scope")
        self.assertIsNone(command({"action": "edited", "issue": issue()}, "issues"))
        self.assertIsNone(command({"action": "opened", "issue": {**issue(), "pull_request": {}}}, "issues"))

    def test_command_scope_and_permissions_boundary(self):
        e = {"action": "created", "issue": issue(), "comment": {"body": "/create-plan", "user": {"type": "User", "login": "qa"}}}
        self.assertEqual(command(e, "issue_comment"), "create-plan")
        e["comment"]["body"] = "/create-plan now"
        self.assertIsNone(command(e, "issue_comment"))
        e["issue"]["pull_request"] = {}
        for body in ("/discuss-plan change acceptance", "/discuss-plan\nchange acceptance"):
            e["comment"]["body"] = body
            self.assertEqual(command(e, "issue_comment"), "discuss-plan")
        for body in ("/discuss-plan", "/discuss-planish change", "text /discuss-plan change"):
            e["comment"]["body"] = body
            self.assertIsNone(command(e, "issue_comment"))
        e["comment"]["body"] = "/discuss-plan change"; e["comment"]["user"]["type"] = "Bot"
        self.assertIsNone(command(e, "issue_comment"))
        class API:
            def request(self, path):
                if not path.endswith("/permission"):
                    raise AssertionError("Read requester reached task data")
                return {"permission": "read"}
        e["comment"]["user"]["type"] = "User"
        self.assertIsNone(github_plan.selected(API(), e))

    def test_planning_context_keeps_issue_and_plan_pr_discussion(self):
        class API:
            def request(self, path, raw=False):
                if path.startswith("/contents/"):
                    return {"encoding": "base64", "size": 4, "content": base64.b64encode(b"Plan").decode()}
                if path.startswith("/tarball/"):
                    return b"archive"
                raise AssertionError(path)
            def pages(self, path):
                return {"/issues/7/comments": [{"id": 1, "body": "Initial detail"}, {"id": 2, "body": "Later correction"}],
                        "/pulls?state=open": [], "/issues/10/comments": [{"id": 3, "body": "Plan discussion"}],
                        "/pulls/10/reviews": [{"id": 4, "body": "Review summary"}],
                        "/pulls/10/comments": [{"id": 5, "body": "Inline"}, {"id": 6, "in_reply_to_id": 5, "body": "Reply"}]}[path]
        for mode in ("scope", "discuss-plan"):
            pr = {"number": 10, "head": {"sha": "a" * 40, "ref": identity(7, "handle_empty_prediction")[0]}} if mode == "discuss-plan" else None
            payload = {"comment": {"id": 8, "body": "/discuss-plan refine"}} if pr else {}
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as folder:
                previous = Path.cwd()
                try:
                    os.chdir(folder)
                    with patch.dict(os.environ, {"GITHUB_OUTPUT": str(Path(folder) / "output")}), patch.object(github_plan, "GitHub", return_value=API()), patch.object(github_plan, "event", return_value=payload), patch.object(github_plan, "selected", return_value=(mode, issue(), pr)), patch.object(github_plan, "default_source", return_value=("main", "a" * 40)), patch.object(Path, "read_text", return_value="Trusted policy"):
                        github_plan.prepare()
                    c = json.loads(Path("context.json").read_text())
                    self.assertEqual(len(c["issue_comments"]), 2)
                    if pr:
                        self.assertEqual(c["pr_reviews"][0]["body"], "Review summary")
                        self.assertEqual(c["pr_review_comments"][1]["in_reply_to_id"], 5)
                        self.assertEqual(c["branch"], pr["head"]["ref"])
                finally:
                    os.chdir(previous)

    def test_model_cannot_select_repository_write_path(self):
        class API:
            calls = []
            def request(self, path, body=None):
                self.calls.append((path, body))
                if path.startswith("/git/commits/"):
                    return {"tree": {"sha": "b" * 40}}
                return {"sha": "c" * 40}
        api = API()
        with self.assertRaises(ValueError):
            github_plan.write_plan_commit(api, "a" * 40, "api/main.go", "malicious", 10)
        self.assertEqual(api.calls, [])
        path = identity(7)[1]
        github_plan.write_plan_commit(api, "a" * 40, path, "plan", 10)
        tree = next(body for endpoint, body in api.calls if endpoint == "/git/trees")
        self.assertEqual([e["path"] for e in tree["tree"]], [path])
        commit = api.calls[-1][1]
        self.assertEqual(commit["parents"], ["a" * 40])

    def test_stale_issue_never_publishes(self):
        changed = {**issue(), "body": "Changed requirement"}
        class API:
            def request(self, *args, **kwargs):
                raise AssertionError("Stale Issue must not write or fetch source")
        with tempfile.TemporaryDirectory() as folder:
            previous = Path.cwd()
            try:
                os.chdir(folder)
                Path("plan-result.json").write_text(json.dumps({"context": context(), "report": report()}))
                with patch.object(github_plan, "GitHub", return_value=API()), patch.object(github_plan, "event", return_value={"issue": issue()}), patch.object(github_plan, "selected", return_value=("scope", changed, None)):
                    with self.assertRaisesRegex(ValueError, "changed"):
                        github_plan.publish()
            finally:
                os.chdir(previous)

    def test_create_plan_publishes_only_markdown_and_draft_refs_pr(self):
        self.exercise_publication("create-plan")

    def test_discuss_plan_uses_fast_forward_and_same_fixed_file(self):
        self.exercise_publication("discuss-plan")

    def test_legacy_plan_publication_keeps_exact_branch(self):
        self.exercise_publication("discuss-plan", "codex/issue-7-plan")

    def exercise_publication(self, mode, branch=None):
        c = context(); c.update(mode=mode, request_id=3, request_body="/create-plan" if mode == "create-plan" else "/discuss-plan revise tests")
        pr = None
        if mode == "discuss-plan":
            c["existing_plan"] = "Old plan"; c["pr_number"] = 10; c["branch"] = branch or identity(7, "handle_empty_prediction")[0]
            pr = {"number": 10, "head": {"sha": c["source_sha"], "ref": c["branch"]}}
        class API:
            def __init__(self):
                self.writes = []
            def pages(self, path):
                return []
            def request(self, path, body=None, method=None):
                if body is not None:
                    self.writes.append((path, body, method))
                    if path == "/pulls":
                        return {"number": 10, "html_url": "https://github.com/example/plan/10"}
                    return {"sha": "c" * 40}
                if path.startswith("/git/ref/"):
                    raise urllib.error.HTTPError("https://api.github.com", 404, "Absent", {}, None)
                if path.startswith("/git/commits/"):
                    return {"tree": {"sha": "b" * 40}, "message": "Existing commit"}
                raise AssertionError("Unexpected API read " + path)
        api = API()
        payload = {"issue": {"number": 10 if pr else 7}, "comment": {"id": 3, "body": c["request_body"]}}
        with tempfile.TemporaryDirectory() as folder:
            previous = Path.cwd()
            try:
                os.chdir(folder)
                Path("plan-result.json").write_text(json.dumps({"context": c, "report": report()}))
                with patch.object(github_plan, "GitHub", return_value=api), patch.object(github_plan, "event", return_value=payload), patch.object(github_plan, "selected", return_value=(mode, issue(), pr)), patch.object(github_plan, "default_source", return_value=("main", "a" * 40)):
                    github_plan.publish()
            finally:
                os.chdir(previous)
        commit = next(body for path, body, method in api.writes if path == "/git/commits")
        self.assertTrue(commit["message"].startswith("chore: "))
        tree = next(body for path, body, method in api.writes if path == "/git/trees")
        self.assertEqual(len(tree["tree"]), 1)
        self.assertEqual(tree["tree"][0]["path"], identity(7)[1])
        if mode == "create-plan":
            request = next(body for path, body, method in api.writes if path == "/pulls")
            self.assertTrue(request["draft"])
            self.assertEqual(request["head"], "runner/fix/handle_empty_prediction/issue_7")
            self.assertIn("Refs #7", request["body"])
            self.assertNotIn("Closes", request["body"])
        else:
            update = next((body, method) for path, body, method in api.writes if path.startswith("/git/refs/heads/"))
            self.assertEqual(update, ({"sha": "c" * 40, "force": False}, "PATCH"))
            self.assertIn(("/git/refs/heads/" + c["branch"], *update), api.writes)
            self.assertFalse(any(path == "/pulls" for path, _, _ in api.writes))

    def test_plan_report_is_bound_to_issue_source_and_safe_paths(self):
        validate_report(report(), context())
        self.assertIn("Граница исправления", render(report(), context()))
        for field, value in (("source_sha", "b" * 40), ("issue_number", 8), ("affected_files", ["../secret"]), ("branch_description", None)):
            r = report(); r[field] = value
            with self.assertRaises(ValueError):
                validate_report(r, context())


if __name__ == "__main__":
    unittest.main()
