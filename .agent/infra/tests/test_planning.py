import base64
import io
import json
import os
import sys
import tempfile
import unittest
import urllib.error
from contextlib import nullcontext, redirect_stdout
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
import github_plan
import ssh_plan
from github_api import GitHub
from plan_policy import command, fingerprint, identity, is_plan_branch, render, validate_report


def issue():
    return {
        "number": 7,
        "id": 70,
        "title": "ML: fix: empty prediction",
        "body": "Return an empty result",
        "state": "open",
    }


def context():
    i = issue()
    return {
        "repository": github_plan.REPOSITORY,
        "mode": "scope",
        "source_sha": "a" * 40,
        "issue_number": 7,
        "issue": i,
        "issue_fingerprint": fingerprint(i),
        "request_id": 70,
        "request_body": "",
        "branch": identity(7)[0],
        "plan_path": identity(7)[1],
        "base_branch": "main",
    }


def report():
    return {
        "source_sha": "a" * 40,
        "issue_number": 7,
        "status": "ready",
        "summary": "Handle an empty prediction",
        "branch_description": "handle_empty_prediction",
        "in_scope": ["Empty input"],
        "out_of_scope": ["Training"],
        "affected_files": ["ml/predict.py"],
        "evidence": ["ml/predict.py:2 indexes the input"],
        "steps": ["Add an empty-input guard"],
        "acceptance": ["Empty input returns empty result"],
        "risks": [],
        "questions": [],
    }


def plan_report():
    value = report()
    del value["steps"]
    return {
        **value,
        "review_mode": "per-stage",
        "stages": [
            {
                "number": 1,
                "title": "Handle empty predictions",
                "result": "Empty input returns an empty result",
                "status": "Не начат",
                "depends_on": [],
                "external_dependencies": [],
                "affected_files": ["ml/predict.py"],
                "actions": ["Add an empty-input guard"],
                "checks": ["Run the empty-input regression test"],
                "acceptance": ["Empty input returns an empty result"],
            }
        ],
    }


class PlanningTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ,
            {
                "GITHUB_EVENT_NAME": "issue_comment",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_RUN_ID": "123",
            },
        )
        environment.start()
        self.addCleanup(environment.stop)

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
        c = context()
        c.update(
            mode="discuss-plan",
            existing_plan="Plan",
            branch=identity(7, "handle_empty_prediction")[0],
        )
        r = report()
        r["branch_description"] = "change_another_thing"
        with self.assertRaisesRegex(ValueError, "existing branch"):
            validate_report(r, c)

    def test_legacy_plan_accepts_discussion_without_renaming(self):
        c = context()
        c.update(mode="discuss-plan", existing_plan="Plan", branch="codex/issue-7-plan")
        self.assertTrue(is_plan_branch(c["branch"], 7))
        self.assertFalse(is_plan_branch(c["branch"], 8))
        validate_report(report(), c)
        payload = {
            "action": "created",
            "issue": {"number": 10, "pull_request": {}, "body": "<!-- agent-plan:issue=7 -->"},
            "comment": {
                "id": 3,
                "body": "/discuss revise plan",
                "user": {"type": "User", "login": "qa"},
            },
        }
        pr = {
            "number": 10,
            "state": "open",
            "body": "<!-- agent-plan:issue=7 -->",
            "head": {"ref": c["branch"], "repo": {"full_name": github_plan.REPOSITORY}},
            "changed_files": 1,
        }

        class API:
            def request(self, path):
                return {
                    "/collaborators/qa/permission": {"permission": "write"},
                    "/issues/comments/3": payload["comment"],
                    "/pulls/10": pr,
                    "/issues/7": issue(),
                }[path]

            def pages(self, path):
                assert path == "/pulls/10/files"
                return [{"filename": c["plan_path"]}]

        self.assertEqual(github_plan.selected(API(), payload), ("discuss-plan", issue(), pr))
        with (
            patch.object(ssh_plan.os, "geteuid", return_value=1000),
            patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": "agent-discuss-plan"}),
            patch.object(ssh_plan.sys, "stdin") as stdin,
            patch.object(ssh_plan, "workspace", return_value=nullcontext(Path("unused"))),
            patch.object(ssh_plan, "infer", return_value=plan_report()) as infer,
            redirect_stdout(io.StringIO()),
        ):
            stdin.buffer = io.BytesIO(json.dumps({"context": c}).encode())
            ssh_plan.main()
        self.assertNotIn("enum", infer.call_args.args[1]["properties"]["branch_description"])

    def test_managed_plans_use_paginated_issue_links_not_repository_history(self):
        def reference(number, repository=github_plan.REPOSITORY):
            return {
                "event": "cross-referenced",
                "source": {
                    "issue": {
                        "number": number,
                        "url": f"https://api.github.com/repos/{repository}/issues/{number}",
                        "pull_request": {},
                    }
                },
            }

        class API(GitHub):
            def __init__(self):
                self.calls = []

            def request(self, path):
                self.calls.append(path)
                if path == "/issues/7/timeline?per_page=100&page=1":
                    return [{"event": "commented"}] * 99 + [reference(10)]
                if path == "/issues/7/timeline?per_page=100&page=2":
                    return [
                        reference(10),
                        reference(11),
                        reference(12),
                        reference(13),
                        reference(14, "other/repo"),
                    ]
                number = int(path.removeprefix("/pulls/"))
                return {
                    "number": number,
                    "state": "closed",
                    "body": "<!-- agent-plan:issue=7 -->" if number != 12 else "Other PR",
                    "head": {
                        "ref": "codex/issue-7-plan"
                        if number == 10
                        else identity(7, "handle_empty_prediction")[0],
                        "repo": {
                            "full_name": github_plan.REPOSITORY if number != 13 else "other/repo"
                        },
                    },
                }

        api = API()
        self.assertEqual([p["number"] for p in github_plan.managed_prs(api, 7)], [10, 11])
        self.assertEqual(api.calls.count("/pulls/10"), 1)
        self.assertNotIn("/pulls/14", api.calls)

    def test_all_new_issues_are_assessed_without_qa_filter(self):
        self.assertEqual(command({"action": "opened", "issue": issue()}, "issues"), "scope")
        self.assertIsNone(command({"action": "edited", "issue": issue()}, "issues"))
        self.assertIsNone(
            command({"action": "opened", "issue": {**issue(), "pull_request": {}}}, "issues")
        )

    def test_command_scope_and_permissions_boundary(self):
        e = {
            "action": "created",
            "issue": issue(),
            "comment": {"body": "/create-plan", "user": {"type": "User", "login": "qa"}},
        }
        self.assertEqual(command(e, "issue_comment"), "create-plan")
        e["comment"]["body"] = "/create-plan now"
        self.assertIsNone(command(e, "issue_comment"))
        e["issue"]["pull_request"] = {}
        e["issue"]["body"] = "<!-- agent-plan:issue=7 -->"
        for body in ("/discuss change plan acceptance", "/discuss\nchange plan acceptance"):
            e["comment"]["body"] = body
            self.assertEqual(command(e, "issue_comment"), "discuss-plan")
        for body in ("/discuss", "/discussish change plan", "text /discuss change plan"):
            e["comment"]["body"] = body
            self.assertIsNone(command(e, "issue_comment"))
        e["comment"]["body"] = "/discuss change plan"
        e["comment"]["user"]["type"] = "Bot"
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
                    return {
                        "encoding": "base64",
                        "size": 4,
                        "content": base64.b64encode(b"Plan").decode(),
                    }
                if path.startswith("/tarball/"):
                    return b"archive"
                raise AssertionError(path)

            def pages(self, path):
                return {
                    "/issues/7/comments": [
                        {"id": 1, "body": "Initial detail"},
                        {"id": 2, "body": "Later correction"},
                    ],
                    "/pulls?state=open": [],
                    "/issues/10/comments": [{"id": 3, "body": "Plan discussion"}],
                    "/pulls/10/reviews": [{"id": 4, "body": "Review summary"}],
                    "/pulls/10/comments": [
                        {"id": 5, "body": "Inline"},
                        {"id": 6, "in_reply_to_id": 5, "body": "Reply"},
                    ],
                }[path]

        for mode in ("scope", "discuss-plan"):
            pr = (
                {
                    "number": 10,
                    "head": {"sha": "a" * 40, "ref": identity(7, "handle_empty_prediction")[0]},
                }
                if mode == "discuss-plan"
                else None
            )
            payload = {"comment": {"id": 8, "body": "/discuss revise plan"}} if pr else {}
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as folder:
                previous = Path.cwd()
                try:
                    os.chdir(folder)
                    with (
                        patch.dict(os.environ, {"GITHUB_OUTPUT": str(Path(folder) / "output")}),
                        patch.object(github_plan, "GitHub", return_value=API()),
                        patch.object(github_plan, "event", return_value=payload),
                        patch.object(github_plan, "selected", return_value=(mode, issue(), pr)),
                        patch.object(
                            github_plan, "default_source", return_value=("main", "a" * 40)
                        ),
                        patch.object(Path, "read_text", return_value="Trusted policy"),
                    ):
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
            github_plan.write_plan_commit(
                api, "a" * 40, "api/main.go", "malicious", "feat: plan fix"
            )
        self.assertEqual(api.calls, [])
        path = identity(7)[1]
        github_plan.write_plan_commit(api, "a" * 40, path, "plan", "feat: plan fix")
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
                Path("plan-result.json").write_text(
                    json.dumps({"context": context(), "report": report()})
                )
                with (
                    patch.object(github_plan, "GitHub", return_value=API()),
                    patch.object(github_plan, "event", return_value={"issue": issue()}),
                    patch.object(github_plan, "selected", return_value=("scope", changed, None)),
                    self.assertRaisesRegex(ValueError, "changed"),
                ):
                    github_plan.publish()
            finally:
                os.chdir(previous)

    def test_create_plan_publishes_only_markdown_and_draft_refs_pr(self):
        self.exercise_publication("create-plan")

    def test_discuss_updates_plan_with_fast_forward_and_same_fixed_file(self):
        self.exercise_publication("discuss-plan")

    def test_discuss_amendment_start_is_idempotent_and_failure_edits_status(self):
        payload = {
            "action": "created",
            "issue": {"number": 10, "pull_request": {}, "body": "<!-- agent-plan:issue=7 -->"},
            "comment": {
                "id": 3,
                "body": "/discuss Поправь план: добавь тест",
                "user": {"type": "User"},
            },
        }

        class API:
            def __init__(self):
                self.comments, self.writes = [], []

            def pages(self, path):
                return self.comments

            def request(self, path, body, method=None):
                self.writes.append((path, body, method))
                if method is None:
                    self.comments.append(
                        {"id": 33, "user": {"login": "github-actions[bot]"}, **body}
                    )

        api = API()
        with (
            patch.object(github_plan, "GitHub", return_value=api),
            patch.object(github_plan, "event", return_value=payload),
            patch.object(
                github_plan, "selected", return_value=("discuss-plan", issue(), {"number": 10})
            ),
            patch.dict(os.environ, {"PLAN_JOB_RESULT": "failure"}),
        ):
            github_plan.start()
            github_plan.start()
            github_plan.publish()
        self.assertEqual(len(api.writes), 2)
        self.assertEqual(api.writes[-1][0], "/issues/comments/33")
        self.assertEqual(api.writes[-1][2], "PATCH")
        self.assertIn("ошибкой", api.writes[-1][1]["body"])

    def test_legacy_plan_publication_keeps_exact_branch(self):
        self.exercise_publication("discuss-plan", "codex/issue-7-plan")

    def test_discuss_reuses_commit_with_exact_request_link(self):
        self.exercise_publication(
            "discuss-plan",
            existing_message=f"feat: update fix plan\n\nRequest: https://github.com/{github_plan.REPOSITORY}/pull/10#issuecomment-3",
            expect_commit=False,
        )

    def test_discuss_does_not_confuse_another_request_link(self):
        self.exercise_publication(
            "discuss-plan",
            existing_message=f"feat: update fix plan\n\nRequest: https://github.com/{github_plan.REPOSITORY}/pull/10#issuecomment-30",
        )

    def exercise_publication(
        self, mode, branch=None, existing_message="Existing commit", expect_commit=True
    ):
        c = context()
        c.update(
            mode=mode,
            request_id=3,
            request_body="/create-plan" if mode == "create-plan" else "/discuss revise plan tests",
        )
        pr = None
        if mode == "discuss-plan":
            c["existing_plan"] = "Old plan"
            c["pr_number"] = 10
            c["branch"] = branch or identity(7, "handle_empty_prediction")[0]
            pr = {"number": 10, "head": {"sha": c["source_sha"], "ref": c["branch"]}}

        class API:
            def __init__(self):
                self.writes = []

            def pages(self, path):
                if path == "/issues/10/comments":
                    return [
                        {
                            "id": 33,
                            "user": {"login": "github-actions[bot]"},
                            "body": "<!-- agent-plan-discussion:3 -->\n\nStarted",
                        }
                    ]
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
                    return {"tree": {"sha": "b" * 40}, "message": existing_message}
                if path.startswith("/contents/"):
                    return {
                        "encoding": "base64",
                        "size": 8,
                        "content": base64.b64encode(b"Old plan").decode(),
                    }
                raise AssertionError("Unexpected API read " + path)

        api = API()
        payload = {
            "issue": {"number": 10 if pr else 7},
            "comment": {"id": 3, "body": c["request_body"], "user": {"login": "qa"}},
        }
        with tempfile.TemporaryDirectory() as folder:
            previous = Path.cwd()
            try:
                os.chdir(folder)
                Path("plan-result.json").write_text(
                    json.dumps({"context": c, "report": plan_report()})
                )
                with (
                    patch.object(github_plan, "GitHub", return_value=api),
                    patch.object(github_plan, "event", return_value=payload),
                    patch.object(github_plan, "selected", return_value=(mode, issue(), pr)),
                    patch.object(github_plan, "default_source", return_value=("main", "a" * 40)),
                ):
                    github_plan.publish()
            finally:
                os.chdir(previous)
        if not expect_commit:
            self.assertEqual(len(api.writes), 1)
            self.assertEqual(api.writes[0][0], "/issues/comments/33")
            self.assertEqual(api.writes[0][2], "PATCH")
            return
        commit = next(body for path, body, method in api.writes if path == "/git/commits")
        verb = "create" if mode == "create-plan" else "update"
        subject = f"feat: {verb} fix plan for #7: handle empty prediction"
        self.assertEqual(commit["message"].splitlines()[0], subject)
        self.assertTrue(commit["message"].isascii())
        self.assertIn("\nRefs #7\n", commit["message"])
        route = "pull/10" if pr else "issues/7"
        url = f"https://github.com/{github_plan.REPOSITORY}/{route}#issuecomment-3"
        self.assertIn("Request: " + url, commit["message"].splitlines())
        self.assertIn(
            "Requested by @qa via " + ("/discuss" if pr else "/create-plan"), commit["message"]
        )
        self.assertNotIn("Closes", commit["message"])
        tree = next(body for path, body, method in api.writes if path == "/git/trees")
        self.assertEqual(len(tree["tree"]), 1)
        self.assertEqual(tree["tree"][0]["path"], identity(7)[1])
        self.assertIn("## Этап 1", tree["tree"][0]["content"])
        self.assertIn("Статус: Не начат.", tree["tree"][0]["content"])
        self.assertIn(r"- [ ] Run the empty\-input regression test", tree["tree"][0]["content"])
        self.assertNotIn("- [x]", tree["tree"][0]["content"])
        if mode == "create-plan":
            request = next(body for path, body, method in api.writes if path == "/pulls")
            self.assertTrue(request["draft"])
            self.assertEqual(request["title"], "ML: " + subject)
            self.assertIn(f"[комментарием @qa]({url})", request["body"])
            self.assertEqual(request["head"], "runner/fix/handle_empty_prediction/issue_7")
            self.assertIn("Refs #7", request["body"])
            self.assertNotIn("Closes", request["body"])
        else:
            update = next(
                (body, method)
                for path, body, method in api.writes
                if path.startswith("/git/refs/heads/")
            )
            self.assertEqual(update, ({"sha": "c" * 40, "force": False}, "PATCH"))
            self.assertIn(("/git/refs/heads/" + c["branch"], *update), api.writes)
            self.assertFalse(any(path == "/pulls" for path, _, _ in api.writes))

    def test_plan_report_is_bound_to_issue_source_and_safe_paths(self):
        validate_report(report(), context())
        self.assertIn("Граница исправления", render(report(), context()))
        for field, value in (
            ("source_sha", "b" * 40),
            ("issue_number", 8),
            ("affected_files", ["../secret"]),
            ("branch_description", None),
        ):
            r = report()
            r[field] = value
            with self.assertRaises(ValueError):
                validate_report(r, context())

    def test_source_references_link_to_exact_commit_files_and_line_ranges(self):
        r = report()
        r["evidence"] = [
            ".agent/infra/planning/plan_policy.py:58–60: exact comparison",
            "`.agent/infra/planning/github_plan.py:89–107,114-127`: collection",
            ".github/workflows/issue-planning.yml:13-17 and AGENTS.md:1",
            "ml/predict.py:2 returns a result",
        ]
        markdown = render(r, context())
        base = f"https://github.com/{github_plan.REPOSITORY}/blob/" + "a" * 40
        for path, anchor in (
            (".agent/infra/planning/plan_policy.py", "#L58-L60"),
            (".agent/infra/planning/github_plan.py", "#L89-L107"),
            (".agent/infra/planning/github_plan.py", "#L114-L127"),
            (".github/workflows/issue-planning.yml", "#L13-L17"),
            ("AGENTS.md", "#L1"),
            ("ml/predict.py", "#L2"),
        ):
            with self.subTest(path=path, anchor=anchor):
                self.assertIn(f"]({base}/{path}{anchor})", markdown)
        self.assertIn("exact comparison", markdown)
        self.assertIn("collection", markdown)
        self.assertNotIn("/blob/main/", markdown)

    def test_render_keeps_existing_links_and_invalid_references_unchanged(self):
        r = report()
        existing = "[plan_policy.py:58](https://example.test/already-linked)"
        invalid = "../secret.py:1, file.py:0, file.py:9-2, INFRA:123"
        r["evidence"] = [existing, invalid, "ml/predict.py:2 returns a result"]
        markdown = render(r, context())
        self.assertIn(existing, markdown)
        self.assertIn(invalid, markdown)
        self.assertIn("/ml/predict.py#L2)", markdown)


if __name__ == "__main__":
    unittest.main()
