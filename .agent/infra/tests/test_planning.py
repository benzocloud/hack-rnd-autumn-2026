import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
import urllib.error
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
from plan_policy import command, identity, fingerprint, validate_report, render
import github_plan


def issue():
    return {"number": 7, "id": 70, "title": "ML: fix: empty prediction", "body": "Return an empty result", "state": "open"}


def context():
    i = issue()
    return {"repository": github_plan.REPOSITORY, "mode": "scope", "source_sha": "a" * 40,
            "issue_number": 7, "issue": i, "issue_fingerprint": fingerprint(i),
            "request_id": 70, "request_body": "", "branch": identity(7)[0], "plan_path": identity(7)[1], "base_branch": "main"}


def report():
    return {"source_sha": "a" * 40, "issue_number": 7, "status": "ready", "summary": "Handle an empty prediction",
            "in_scope": ["Empty input"], "out_of_scope": ["Training"], "affected_files": ["ml/predict.py"],
            "evidence": ["ml/predict.py:2 indexes the input"], "steps": ["Add an empty-input guard"],
            "acceptance": ["Empty input returns empty result"], "risks": [], "questions": []}


class PlanningTests(unittest.TestCase):
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

    def exercise_publication(self, mode):
        c = context(); c.update(mode=mode, request_id=3, request_body="/create-plan" if mode == "create-plan" else "/discuss-plan revise tests")
        pr = None
        if mode == "discuss-plan":
            c["existing_plan"] = "Old plan"; c["pr_number"] = 10
            pr = {"number": 10, "head": {"sha": c["source_sha"]}}
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
            self.assertIn("Refs #7", request["body"])
            self.assertNotIn("Closes", request["body"])
        else:
            update = next((body, method) for path, body, method in api.writes if path.startswith("/git/refs/heads/"))
            self.assertEqual(update, ({"sha": "c" * 40, "force": False}, "PATCH"))
            self.assertFalse(any(path == "/pulls" for path, _, _ in api.writes))

    def test_plan_report_is_bound_to_issue_source_and_safe_paths(self):
        validate_report(report(), context())
        self.assertIn("Граница исправления", render(report(), context()))
        for field, value in (("source_sha", "b" * 40), ("issue_number", 8), ("affected_files", ["../secret"])):
            r = report(); r[field] = value
            with self.assertRaises(ValueError):
                validate_report(r, context())


if __name__ == "__main__":
    unittest.main()
