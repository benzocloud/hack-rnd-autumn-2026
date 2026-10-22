"""Exercise planning adapters together; GitHub, SSH and model calls stay mocked."""

import base64
import copy
import io
import json
import os
import sys
import unittest
import urllib.error
from contextlib import nullcontext, redirect_stdout
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "planning"))
import github_plan
import ssh_plan
from plan_policy import identity, previous_plan, render_staged_plan
from test_planning import issue, plan_report
from test_review_context import temporary_workdir


class PlanningAPI:
    def __init__(self):
        self.base = "a" * 40
        self.pr = None
        self.comments = {7: [], 10: []}
        self.commits = {self.base: {"tree": {"sha": "base-tree"}, "message": "Base"}}
        self.trees = {}
        self.refs = {}
        self.writes = []

    def request_event(self, number, body):
        comment = {
            "id": 100 + sum(len(items) for items in self.comments.values()),
            "body": body,
            "user": {"login": "developer", "type": "User"},
        }
        self.comments[number].append(comment)
        task = issue() if number == 7 else {**self.pr, "pull_request": {}}
        return {
            "action": "created",
            "issue": copy.deepcopy(task),
            "comment": copy.deepcopy(comment),
        }

    def request(self, path, body=None, raw=False, method=None):
        if body is not None:
            self.writes.append((path, copy.deepcopy(body), method))
            if path == "/git/trees":
                assert [item["path"] for item in body["tree"]] == [identity(7)[1]]
                sha = f"tree-{len(self.trees)}"
                self.trees[sha] = body["tree"][0]["content"]
                return {"sha": sha}
            if path == "/git/commits":
                sha = f"{len(self.commits):040x}"
                self.commits[sha] = {**body, "tree": {"sha": body["tree"]}}
                return {"sha": sha}
            if path == "/git/refs":
                self.refs[body["ref"].removeprefix("refs/heads/")] = body["sha"]
                return {}
            if path.startswith("/git/refs/heads/"):
                branch = path.removeprefix("/git/refs/heads/")
                assert method == "PATCH" and body["force"] is False
                assert self.commits[body["sha"]]["parents"] == [self.refs[branch]]
                self.refs[branch] = body["sha"]
                self.pr["head"]["sha"] = body["sha"]
                return {}
            if path == "/pulls":
                self.pr = {
                    **body,
                    "number": 10,
                    "state": "open",
                    "changed_files": 1,
                    "html_url": f"https://github.com/{github_plan.REPOSITORY}/pull/10",
                    "head": {
                        "sha": self.refs[body["head"]],
                        "ref": body["head"],
                        "repo": {"full_name": github_plan.REPOSITORY},
                    },
                }
                return self.pr
            if path in ("/issues/7/comments", "/issues/10/comments"):
                number = int(path.split("/")[2])
                comment = {
                    "id": 1000 + len(self.writes),
                    **body,
                    "user": {"login": "github-actions[bot]", "type": "Bot"},
                }
                self.comments[number].append(comment)
                return comment
            if path.startswith("/issues/comments/"):
                assert method == "PATCH"
                comment = self.comment(int(path.split("/")[-1]))
                comment.update(body)
                return comment
            raise AssertionError("Unexpected write: " + path)
        if path == "":
            return {"default_branch": "main"}
        if path == "/commits/main":
            return {"sha": self.base}
        if path.endswith("/permission"):
            return {"permission": "write"}
        if path.startswith("/issues/comments/"):
            return self.comment(int(path.split("/")[-1]))
        if path == "/issues/7":
            return issue()
        if path == "/pulls/10":
            return self.pr
        if path.startswith("/tarball/"):
            assert raw
            return b"source snapshot fixture"
        if path.startswith("/git/commits/"):
            return self.commits[path.split("/")[-1]]
        if path.startswith("/git/ref/heads/"):
            raise urllib.error.HTTPError("https://api.github.test", 404, "Absent", {}, None)
        if path.startswith("/contents/"):
            assert path.split("?", 1)[0] == "/contents/" + identity(7)[1]
            markdown = self.markdown(path.split("?ref=")[1])
            return {
                "encoding": "base64",
                "size": len(markdown.encode()),
                "content": base64.b64encode(markdown.encode()).decode(),
            }
        raise AssertionError("Unexpected read: " + path)

    def comment(self, number):
        return next(c for items in self.comments.values() for c in items if c["id"] == number)

    def markdown(self, sha):
        return self.trees[self.commits[sha]["tree"]["sha"]]

    def pages(self, path):
        if path == "/pulls?state=open":
            return copy.deepcopy([self.pr] if self.pr else [])
        if path == "/issues/7/timeline":
            return (
                [
                    {
                        "event": "cross-referenced",
                        "source": {
                            "issue": {
                                "number": 10,
                                "pull_request": {},
                                "url": f"https://api.github.com/repos/{github_plan.REPOSITORY}/issues/10",
                            }
                        },
                    }
                ]
                if self.pr
                else []
            )
        if path in ("/issues/7/comments", "/issues/10/comments"):
            return copy.deepcopy(self.comments[int(path.split("/")[2])])
        if path == "/pulls/10/files":
            return [{"filename": identity(7)[1]}]
        if path == "/pulls/10/reviews":
            return [{"id": 50, "body": "Keep the accepted scope", "state": "COMMENTED"}]
        if path == "/pulls/10/comments":
            return [
                {"id": 51, "body": "Check empty input"},
                {"id": 52, "body": "Keep nonempty input unchanged", "in_reply_to_id": 51},
            ]
        raise AssertionError("Unexpected pages: " + path)


class PlanningCycleTests(unittest.TestCase):
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

    def cycle(self, api, payload, expected, folder, before_publish=None):
        for name in (
            "AGENTS.md",
            ".agent/instructions.md",
            ".agent/decisions.md",
            ".agent/workflows/issue-planning.md",
            ".agent/workflows/context-and-memory.md",
            ".agent/workflows/git-conventions.md",
            ".agent/references/go-services.md",
        ):
            path = folder / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("Trusted policy fixture")

        def bridge(command):
            package = json.loads(Path("request.json").read_text())
            self.assertEqual(command, "agent-" + package["context"]["mode"])
            self.assertEqual(base64.b64decode(package["archive"]), b"source snapshot fixture")
            self.assertNotIn("GH_TOKEN", package)
            with (
                patch.object(ssh_plan.os, "geteuid", return_value=1000),
                patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": command}),
                patch.object(ssh_plan.sys, "stdin") as stdin,
                patch.object(ssh_plan, "workspace", return_value=nullcontext(folder)),
                patch.object(ssh_plan, "infer", return_value=expected) as infer,
                redirect_stdout(io.StringIO()) as stdout,
            ):
                stdin.buffer = io.BytesIO(json.dumps(package).encode())
                ssh_plan.main()
                self.assertIn("stages", infer.call_args.args[1]["required"])
                self.assertEqual(
                    infer.call_args.args[1]["properties"]["review_mode"]["enum"],
                    [expected["review_mode"]],
                )
            return json.loads(stdout.getvalue())

        with (
            patch.dict(os.environ, {"GITHUB_OUTPUT": str(folder / "outputs")}),
            patch.object(github_plan, "GitHub", return_value=api),
            patch.object(github_plan, "event", return_value=payload),
            patch.object(github_plan, "invoke_json", side_effect=bridge) as invoke,
            patch("subprocess.run", side_effect=AssertionError("No real model/SSH calls allowed")),
            patch("urllib.request.urlopen", side_effect=AssertionError("No real HTTP allowed")),
        ):
            github_plan.authorize()
            if "pull_request" in payload["issue"]:
                github_plan.start()
            github_plan.prepare()
            collected = json.loads(Path("context.json").read_text())
            github_plan.invoke()
            invoke.assert_called_once()
            if before_publish:
                before_publish()
            github_plan.publish()
        return collected

    def test_create_and_revise_preserve_modes_history_and_publication_boundaries(self):
        for review_mode in ("per-stage", "after-all"):
            with self.subTest(review_mode=review_mode), temporary_workdir() as folder:
                api = PlanningAPI()
                first = plan_report()
                self.cycle(api, api.request_event(7, "/create-plan"), first, folder)
                self.assertTrue(api.pr["draft"])
                self.assertIn("Refs #7", api.pr["body"])
                self.assertEqual(len([p for p, _, _ in api.writes if p == "/pulls"]), 1)
                self.assertIn(api.pr["html_url"], api.comments[7][-1]["body"])

                previous = copy.deepcopy(first)
                if review_mode == "after-all":
                    # Emulate a developer-committed plan state, not a model decision.
                    previous["review_mode"] = review_mode
                    previous["stages"][0]["status"] = "Принят"
                    sha = github_plan.write_plan_commit(
                        api,
                        api.pr["head"]["sha"],
                        identity(7)[1],
                        render_staged_plan(previous),
                        "docs: record developer-approved plan state",
                    )
                    api.request(
                        "/git/refs/heads/" + api.pr["head"]["ref"],
                        {"sha": sha, "force": False},
                        method="PATCH",
                    )

                updated = copy.deepcopy(previous)
                updated["source_sha"] = api.pr["head"]["sha"]
                updated["stages"].append(
                    {
                        **copy.deepcopy(first["stages"][0]),
                        "number": 2,
                        "title": "Add regression coverage",
                        "depends_on": [1],
                    }
                )
                payload = api.request_event(10, "/discuss Поправь план: добавь этап проверки")
                collected = self.cycle(api, payload, updated, folder)
                self.assertEqual(collected["pr_review_comments"][-1]["in_reply_to_id"], 51)
                self.assertEqual(collected["pr_reviews"][0]["body"], "Keep the accepted scope")
                markdown = api.markdown(api.pr["head"]["sha"])
                saved = previous_plan({**collected, "existing_plan": markdown})
                self.assertEqual(saved, updated)
                self.assertEqual(saved["stages"][0], previous["stages"][0])
                self.assertEqual(saved["stages"][1]["status"], "Не начат")
                self.assertNotIn("- [x]", markdown)
                self.assertIn("План обновлён", api.comments[10][-1]["body"])
                self.assertEqual(len([p for p, _, _ in api.writes if p == "/pulls"]), 1)
                self.assertEqual(api.writes[-1][2], "PATCH")

                # A repeated create request reuses detection without spending inference.
                payload = api.request_event(7, "/create-plan")
                with (
                    patch.object(github_plan, "GitHub", return_value=api),
                    patch.object(github_plan, "event", return_value=payload),
                    patch.dict(os.environ, {"GITHUB_OUTPUT": str(folder / "duplicate-output")}),
                    patch.object(github_plan, "invoke_json") as invoke,
                    redirect_stdout(io.StringIO()),
                ):
                    github_plan.prepare()
                invoke.assert_not_called()
                self.assertEqual((folder / "duplicate-output").read_text(), "requested=false\n")

    def test_source_change_during_analysis_aborts_before_git_writes(self):
        with temporary_workdir() as folder:
            api = PlanningAPI()
            with self.assertRaisesRegex(ValueError, "Source changed"):
                self.cycle(
                    api,
                    api.request_event(7, "/create-plan"),
                    plan_report(),
                    folder,
                    before_publish=lambda: setattr(api, "base", "d" * 40),
                )
            self.assertEqual(api.writes, [])


if __name__ == "__main__":
    unittest.main()
