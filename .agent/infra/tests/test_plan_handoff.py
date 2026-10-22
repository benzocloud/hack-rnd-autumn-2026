"""Plan handoff publication and retry paths; no GitHub, SSH or model calls."""

import copy
import io
import os
import sys
import unittest
import urllib.error
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
import github_plan
import test_planning_cycle as cycle
from plan_policy import identity
from test_planning import plan_report
from test_review_context import temporary_workdir


class API(cycle.PlanningAPI):
    def __init__(self):
        super().__init__()
        self.fail_write = None
        self.plan_content = "normal"
        self.file_status = "added"
        self.after_link = None
        self.unconfirmed_close = False
        self.attempts = []

    def request(self, path, body=None, raw=False, method=None):
        if body is not None:
            self.attempts.append((path, copy.deepcopy(body), method))
            if path == self.fail_write:
                raise RuntimeError("Simulated publication failure: " + path)
            if path == "/issues/7" and self.unconfirmed_close:
                return {"number": 7, "state": "open"}
        if body is None and path.startswith("/git/ref/heads/"):
            branch = path.removeprefix("/git/ref/heads/")
            if branch in self.refs:
                return {"object": {"sha": self.refs[branch]}}
        if body is None and path.startswith("/contents/"):
            if self.plan_content == "missing":
                raise urllib.error.HTTPError(
                    "https://api.github.test", 404, "Missing plan", {}, None
                )
            if self.plan_content == "empty":
                return {"encoding": "base64", "size": 0, "content": ""}
        result = super().request(path, body=body, raw=raw, method=method)
        if body is not None and path == "/issues/7/comments" and self.after_link:
            self.after_link()
        return result

    def pages(self, path):
        if path == "/pulls/10/files":
            return [{"filename": identity(7)[1], "status": self.file_status}]
        return super().pages(path)


class PlanHandoffTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ,
            {
                "GITHUB_EVENT_NAME": "issue_comment",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_RUN_ID": "123",
                "PLAN_JOB_RESULT": "success",
                "PLAN_REQUESTED": "true",
            },
        )
        environment.start()
        self.addCleanup(environment.stop)

    def create(self, api, folder):
        return cycle.PlanningCycleTests.cycle(
            self,
            api,
            api.request_event(7, "/create-plan"),
            plan_report(),
            folder,
        )

    def reuse(self, api, folder):
        payload = api.request_event(7, "/create-plan")
        output = folder / "reuse-output"
        output.unlink(missing_ok=True)
        # Existing-plan handoff cannot consume an old inference artifact.
        Path("plan-result.json").write_text("obsolete artifact, not JSON")
        with (
            patch.object(github_plan, "GitHub", return_value=api),
            patch.object(github_plan, "event", return_value=payload),
            patch.dict(os.environ, {"GITHUB_OUTPUT": str(output), "PLAN_REQUESTED": "false"}),
            patch.object(
                github_plan, "invoke_json", side_effect=AssertionError("No inference on reuse")
            ) as invoke,
            redirect_stdout(io.StringIO()),
        ):
            github_plan.prepare()
            self.assertEqual(output.read_text(), "requested=false\n")
            github_plan.publish()
        invoke.assert_not_called()

    def test_creation_links_a_verified_plan_before_closing_the_issue(self):
        with temporary_workdir() as folder:
            api = API()
            self.create(api, folder)
            paths = [path for path, _, _ in api.writes]
            self.assertLess(paths.index("/pulls"), paths.index("/issues/7/comments"))
            self.assertLess(paths.index("/issues/7/comments"), paths.index("/issues/7"))
            self.assertEqual(api.writes[-1], ("/issues/7", {"state": "closed"}, "PATCH"))
            self.assertEqual(api.issue["state"], "closed")
            self.assertIn(api.pr["html_url"], api.comments[7][-1]["body"])
            self.assertIn("а не готовность исправления", api.comments[7][-1]["body"])

    def test_failed_creation_or_link_never_closes_the_issue(self):
        for path in ("/pulls", "/issues/7/comments"):
            with self.subTest(path=path), temporary_workdir() as folder:
                api = API()
                api.fail_write = path
                with self.assertRaisesRegex(RuntimeError, "publication failure"):
                    self.create(api, folder)
                self.assertEqual(api.issue["state"], "open")
                self.assertFalse(any(p == "/issues/7" for p, _, _ in api.attempts))

    def test_partial_failure_reuses_pr_and_link_without_inference_or_git_writes(self):
        for path in ("/issues/7/comments", "/issues/7"):
            with self.subTest(path=path), temporary_workdir() as folder:
                api = API()
                api.fail_write = path
                with self.assertRaises(RuntimeError):
                    self.create(api, folder)
                self.assertIsNotNone(api.pr)
                self.assertEqual(api.issue["state"], "open")
                head = api.pr["head"]["sha"]
                before = len(api.writes)
                api.fail_write = None
                self.reuse(api, folder)
                self.assertEqual(api.issue["state"], "closed")
                self.assertEqual(api.pr["head"]["sha"], head)
                self.assertFalse(
                    any(p.startswith("/git/") or p == "/pulls" for p, _, _ in api.writes[before:])
                )
                handoffs = [
                    c
                    for c in api.comments[7]
                    if c.get("body", "").startswith("<!-- agent-plan-handoff:")
                ]
                self.assertEqual(len(handoffs), 1)
                self.assertIn(api.pr["html_url"], handoffs[0]["body"])

    def test_reopened_issue_reuses_the_existing_handoff_comment(self):
        with temporary_workdir() as folder:
            api = API()
            self.create(api, folder)
            before = len(api.writes)
            api.issue["state"] = "open"
            self.reuse(api, folder)
            self.assertEqual(api.writes[before:], [("/issues/7", {"state": "closed"}, "PATCH")])

    def test_removed_missing_or_empty_plan_never_gets_handed_off(self):
        for state in ("removed", "missing", "empty"):
            with self.subTest(state=state), temporary_workdir() as folder:
                api = API()
                self.create(api, folder)
                api.issue["state"] = "open"
                api.comments[7] = [
                    c
                    for c in api.comments[7]
                    if not c.get("body", "").startswith("<!-- agent-plan-handoff:")
                ]
                if state == "removed":
                    api.file_status = state
                else:
                    api.plan_content = state
                before = len(api.writes)
                with self.assertRaises((ValueError, urllib.error.HTTPError)):
                    self.reuse(api, folder)
                self.assertEqual(api.writes[before:], [])
                self.assertEqual(api.issue["state"], "open")

    def test_closed_or_implementation_plan_pr_leaves_reopened_issue_open(self):
        for state in ("closed", "implementation"):
            with self.subTest(state=state), temporary_workdir() as folder:
                api = API()
                self.create(api, folder)
                api.issue["state"] = "open"
                if state == "closed":
                    api.pr["state"] = "closed"
                else:
                    api.pr["changed_files"] = 2
                before = len(api.writes)
                with self.assertRaises(ValueError):
                    self.reuse(api, folder)
                self.assertEqual(api.writes[before:], [])
                self.assertEqual(api.issue["state"], "open")

    def test_issue_or_pr_changes_after_link_stop_closure(self):
        for state in ("issue", "pr"):
            with self.subTest(state=state), temporary_workdir() as folder:
                api = API()

                def change(api=api, state=state):
                    if state == "issue":
                        api.issue["body"] += " New requirement"
                    else:
                        api.pr["head"]["sha"] = "d" * 40

                api.after_link = change
                with self.assertRaisesRegex(ValueError, "changed"):
                    self.create(api, folder)
                self.assertEqual(api.issue["state"], "open")
                self.assertIn(api.pr["html_url"], api.comments[7][-1]["body"])
                self.assertFalse(any(p == "/issues/7" for p, _, _ in api.attempts))

    def test_corrupted_link_comment_is_repaired_before_closure(self):
        for corrupted in (
            "Missing link",
            "[Other PR](https://github.com/benzocloud/hack-rnd-autumn-2026/pull/100)",
        ):
            with self.subTest(corrupted=corrupted), temporary_workdir() as folder:
                api = API()
                self.create(api, folder)
                handoff = api.comments[7][-1]
                handoff["body"] = handoff["body"].splitlines()[0] + "\n" + corrupted
                api.issue["state"] = "open"
                before = len(api.writes)
                self.reuse(api, folder)
                self.assertEqual(
                    [p for p, _, _ in api.writes[before:]],
                    [f"/issues/comments/{handoff['id']}", "/issues/7"],
                )
                self.assertIn(api.pr["html_url"], handoff["body"])

    def test_unconfirmed_closure_preserves_pr_and_link_for_retry(self):
        with temporary_workdir() as folder:
            api = API()
            api.unconfirmed_close = True
            with self.assertRaisesRegex(ValueError, "not confirmed"):
                self.create(api, folder)
            self.assertEqual(api.issue["state"], "open")
            self.assertIn(api.pr["html_url"], api.comments[7][-1]["body"])
            api.unconfirmed_close = False
            self.reuse(api, folder)
            self.assertEqual(api.issue["state"], "closed")

    def test_orphan_branch_after_failed_pr_creation_is_preserved_for_manual_recovery(self):
        with temporary_workdir() as folder:
            api = API()
            api.fail_write = "/pulls"
            with self.assertRaises(RuntimeError):
                self.create(api, folder)
            refs = copy.deepcopy(api.refs)
            self.assertTrue(refs)
            api.fail_write = None
            before = len(api.writes)
            with self.assertRaisesRegex(ValueError, "without a managed PR"):
                self.create(api, folder)
            self.assertEqual(api.refs, refs)
            self.assertEqual(api.writes[before:], [])
            self.assertEqual(api.issue["state"], "open")

    def test_duplicate_handoff_comments_and_revoked_permission_prevent_closure(self):
        for state in ("duplicate", "unauthorized"):
            with self.subTest(state=state), temporary_workdir() as folder:
                api = API()
                self.create(api, folder)
                api.issue["state"] = "open"
                if state == "duplicate":
                    api.comments[7].append(copy.deepcopy(api.comments[7][-1]))
                original = api.request

                def request(path, *args, original=original, state=state, **kwargs):
                    if state == "unauthorized" and path.endswith("/permission"):
                        return {"permission": "read"}
                    return original(path, *args, **kwargs)

                before = len(api.writes)
                with (
                    patch.object(api, "request", side_effect=request),
                    self.assertRaises(ValueError),
                ):
                    self.reuse(api, folder)
                self.assertEqual(api.writes[before:], [])
                self.assertEqual(api.issue["state"], "open")
