import json
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
import github_plan
from test_planning import context, issue, report
from test_review_context import temporary_workdir


class API:
    def __init__(self):
        self.issue = issue()
        self.comments = []
        self.writes = []

    def request(self, path, body=None, method=None):
        if body is None:
            assert path == "/issues/7", path
            return self.issue
        self.writes.append((path, body, method))
        if path == "/issues/7/comments":
            self.comments.append(
                {"id": 50, "body": body["body"], "user": {"login": "github-actions[bot]"}}
            )
        elif path == "/issues/comments/50" and method == "PATCH":
            self.comments[0]["body"] = body["body"]
        else:
            raise AssertionError(path)
        return self.comments[0]

    def pages(self, path):
        assert path == "/issues/7/comments", path
        return self.comments


class ScopeStatusTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ,
            {
                "GITHUB_EVENT_NAME": "issues",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_RUN_ID": "123",
                "PLAN_JOB_RESULT": "success",
                "PLAN_REQUESTED": "true",
            },
        )
        environment.start()
        self.addCleanup(environment.stop)
        self.api = API()
        self.payload = {"action": "opened", "issue": issue()}
        for name, value in (
            ("GitHub", self.api),
            ("event", self.payload),
            ("default_source", ("main", "a" * 40)),
        ):
            mock = patch.object(github_plan, name, return_value=value)
            mock.start()
            self.addCleanup(mock.stop)

    def test_start_is_idempotent_and_success_edits_the_same_comment(self):
        with temporary_workdir():
            github_plan.start()
            github_plan.start()
            self.assertIn("Начал оценку", self.api.comments[0]["body"])
            Path("plan-result.json").write_text(
                json.dumps({"context": context(), "report": report()})
            )
            github_plan.publish()
        self.assertEqual(
            [p for p, _, _ in self.api.writes], ["/issues/7/comments", "/issues/comments/50"]
        )
        self.assertEqual(self.api.writes[-1][2], "PATCH")
        self.assertIn("Оценка границ исправления завершена", self.api.comments[0]["body"])
        self.assertIn("a" * 40, self.api.comments[0]["body"])
        self.assertIn("/create-plan", self.api.comments[0]["body"])

    def test_failure_cancellation_and_missing_artifact_update_status(self):
        for result, requested in (("failure", "true"), ("cancelled", "true"), ("success", "false")):
            with (
                self.subTest(result=result, requested=requested),
                temporary_workdir(),
                patch.dict(os.environ, {"PLAN_JOB_RESULT": result, "PLAN_REQUESTED": requested}),
            ):
                self.api = API()
                with patch.object(github_plan, "GitHub", return_value=self.api):
                    github_plan.start()
                    github_plan.publish()
                self.assertEqual(len(self.api.writes), 2)
                self.assertIn("Итоговая оценка не опубликована", self.api.comments[0]["body"])
                self.assertIn("actions/runs/123", self.api.comments[0]["body"])

    def test_stale_or_old_runner_report_never_publishes_a_completed_scope(self):
        for stale in (False, True):
            with self.subTest(stale=stale), temporary_workdir():
                self.api = API()
                with patch.object(github_plan, "GitHub", return_value=self.api):
                    github_plan.start()
                    r = report()
                    if stale:
                        self.api.issue = {**issue(), "body": "Changed requirement"}
                    else:
                        r.pop("branch_description")
                    Path("plan-result.json").write_text(
                        json.dumps({"context": context(), "report": r})
                    )
                    with self.assertRaisesRegex(
                        ValueError, "changed" if stale else "deploy matching"
                    ):
                        github_plan.publish()
                self.assertIn("Оценка Issue не опубликована", self.api.comments[0]["body"])
                self.assertEqual(len(self.api.writes), 2)

    def test_missing_start_comment_does_not_create_a_duplicate_result(self):
        with self.assertRaisesRegex(ValueError, "missing"):
            github_plan.scope_status(self.api, self.payload, "Result")
        self.assertEqual(self.api.writes, [])
