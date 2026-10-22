import copy
import io
import json
import os
import sys
import tempfile
import unittest
from contextlib import nullcontext, redirect_stdout
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "conversation"))
import conversation_policy as policy
import github_conversation as github
import job
import review_dispatch
import ssh_conversation as worker
from plan_policy import command as planning_command


def payload(body="/question Почему падает api/main.py?"):
    return {
        "action": "created",
        "issue": {"number": 2},
        "comment": {"id": 30, "body": body, "user": {"login": "qa", "type": "User"}},
    }


class API:
    def __init__(self, pr=False):
        self.permission = "write"
        self.payload = payload()
        self.task = {
            "number": 2,
            "state": "open",
            "title": "INFRA: fix: regression",
            "body": "Refs #7",
        }
        self.pr = None
        if pr:
            self.task["pull_request"] = {}
            self.pr = {
                "number": 2,
                "state": "open",
                "changed_files": 1,
                "head": {"sha": "b" * 40, "repo": {"full_name": github.REPOSITORY}},
                "base": {"sha": "a" * 40},
            }
        self.issue = {"number": 7, "state": "open", "title": "Bug", "body": "Details"}
        self.linked_comments = [{"id": 20, "body": "QA correction"}]
        self.comments = [copy.deepcopy(self.payload["comment"])]
        self.reviews = [{"id": 11, "body": "Review summary"}]
        self.inline = [
            {"id": 12, "body": "Inline"},
            {"id": 13, "body": "Reply", "in_reply_to_id": 12},
        ]
        self.files = [{"filename": "api/main.py"}]
        self.writes = []

    def request(self, path, body=None, method=None, raw=False):
        if body is not None:
            if path.startswith("/git/") or path.startswith("/contents/") or path == "/pulls":
                raise AssertionError("Conversation must never write Git data")
            self.writes.append((path, body, method))
            if method == "PATCH":
                next(c for c in self.comments if c["id"] == 88)["body"] = body["body"]
            else:
                self.comments.append(
                    {"id": 88, "body": body["body"], "user": {"login": "github-actions[bot]"}}
                )
            return {}
        return {
            "": {"default_branch": "main"},
            "/commits/main": {"sha": "a" * 40},
            "/collaborators/qa/permission": {"permission": self.permission},
            "/issues/comments/30": self.payload["comment"],
            "/issues/2": self.task,
            "/pulls/2": self.pr,
            "/issues/7": self.issue,
            "/tarball/" + ("b" if self.pr else "a") * 40: b"archive",
        }[path]

    def pages(self, path):
        return copy.deepcopy(
            {
                "/issues/2/comments": self.comments,
                "/pulls/2/reviews": self.reviews,
                "/pulls/2/comments": self.inline,
                "/pulls/2/files": self.files,
                "/issues/7/comments": self.linked_comments,
            }[path]
        )


def result(context):
    return {
        "report": {
            **{k: context[k] for k in ("source_sha", "thread_number", "comment_id", "mode")},
            "status": "complete",
            "answer": "Ошибка в api/main.py:2-4.",
        },
        "model": policy.MODELS[context["mode"]],
        "reasoning": "low",
        "usage": {"input_tokens": 100, "cached_input_tokens": 10, "output_tokens": 20},
        "source_files": ["api/main.py"],
        "skipped_source_files": [],
    }


class ConversationTests(unittest.TestCase):
    def setUp(self):
        env = patch.dict(
            os.environ,
            {
                "GITHUB_EVENT_NAME": "issue_comment",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_RUN_ID": "123",
            },
        )
        env.start()
        self.addCleanup(env.stop)
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        previous = Path.cwd()
        self.addCleanup(os.chdir, previous)
        os.chdir(directory.name)

    def collect(self, api):
        return github.collect(api, api.payload, github.selected(api, api.payload))

    def start(self, api):
        with (
            patch.object(github, "GitHub", return_value=api),
            patch.object(github, "event", return_value=api.payload),
        ):
            github.start()

    def save(self, context):
        Path("conversation-result.json").write_text(
            json.dumps({"context": context, "result": result(context)})
        )

    def test_only_new_human_nonempty_exact_commands_are_routed(self):
        for body in ("/question explain", "/question\nexplain", "/discuss Explain the plan"):
            self.assertIsNotNone(policy.command(payload(body), "issue_comment"))
        for body in (
            "/question",
            " /question explain",
            "/questionable explain",
            "/discussish change plan",
            "/discuss",
            "/discuss \n",
            "text /discuss explain",
        ):
            self.assertIsNone(policy.command(payload(body), "issue_comment"))
            self.assertIsNone(planning_command(payload(body), "issue_comment"))
        p = payload()
        self.assertIsNone(policy.command(p, "issue_comment", "2"))
        self.assertIsNone(policy.command(p, "pull_request"))
        p["action"] = "edited"
        self.assertIsNone(policy.command(p, "issue_comment"))
        p["action"] = "created"
        p["comment"]["user"]["type"] = "Bot"
        self.assertIsNone(policy.command(p, "issue_comment"))

    def test_only_explicit_amendment_in_managed_plan_pr_enters_write_flow(self):
        for body in (
            "/discuss Поправь план: сначала добавь тест",
            "/discuss Update plan: add tests",
            "/discuss Дополни этот план: добавь риски",
            "/discuss Добавь в план: регрессионные тесты",
            "/discuss Убери из плана: изменение API",
            "/discuss Edit the plan: keep the API unchanged",
            "/discuss Add to the plan: regression tests",
            "/discuss Remove from this plan: the API change",
        ):
            p = payload(body)
            self.assertIsNotNone(policy.command(p, "issue_comment"))
            p["issue"].update(pull_request={}, body="ordinary PR")
            self.assertIsNotNone(policy.command(p, "issue_comment"))
            p["issue"]["body"] = "<!-- agent-plan:issue=7 -->"
            self.assertIsNone(policy.command(p, "issue_comment"))
            self.assertEqual(planning_command(p, "issue_comment"), "discuss-plan")
        for body in (
            "/question Поправь план",
            "/discuss Как поправить план?",
            "/discuss Изменить план?",
            "/discuss Update plan?",
            "/discuss What should we change in the plan?",
            "/discuss Добавь объяснение, зачем менять план",
            "/discuss Добавь комментарий: план менять не надо",
            "/discuss Изменить план\nили сначала обсудить риски?",
            "/discuss Add an explanation of why the plan should stay unchanged",
            "/discuss Explain how to update the plan",
            "/discuss Не меняй план: только объясни решение",
        ):
            p = payload(body)
            p["issue"].update(pull_request={}, body="<!-- agent-plan:issue=7 -->")
            self.assertIsNotNone(policy.command(p, "issue_comment"))
            self.assertIsNone(planning_command(p, "issue_comment"))

    def test_authorization_rejects_readers_changed_commands_closed_and_fork_prs(self):
        api = API(pr=True)
        api.permission = "read"
        self.assertIsNone(github.selected(api, api.payload))
        api.permission = "write"
        stale = copy.deepcopy(api.payload)
        api.payload["comment"]["body"] += " edited"
        with self.assertRaisesRegex(ValueError, "changed"):
            github.selected(api, stale)
        api.pr["head"]["repo"]["full_name"] = "other/repo"
        with self.assertRaisesRegex(ValueError, "same-repository"):
            github.selected(api, api.payload)
        api.pr["head"]["repo"]["full_name"] = github.REPOSITORY
        api.task["state"] = "closed"
        with self.assertRaises(ValueError):
            github.selected(api, api.payload)

    def test_collect_preserves_full_pr_reviews_replies_linked_issues_and_plan(self):
        api = API(pr=True)
        api.task["body"] += "\n<!-- agent-plan:issue=7 -->"
        c = self.collect(api)
        self.assertEqual(c["source_sha"], "b" * 40)
        self.assertEqual(c["pr_reviews"], api.reviews)
        self.assertEqual(c["pr_review_comments"][1]["in_reply_to_id"], 12)
        self.assertEqual(c["issues"][0]["comments"][0]["body"], "QA correction")
        self.assertEqual(c["existing_plan"], [".agent/plans/issue-7-fix.md"])
        api.pr["changed_files"] = 2
        with self.assertRaisesRegex(ValueError, "Incomplete"):
            self.collect(api)
        with (
            patch.object(api, "pages", side_effect=ValueError("Pagination failed")),
            self.assertRaisesRegex(ValueError, "Pagination"),
        ):
            self.collect(api)

    def test_pr_mentions_are_not_treated_as_issue_tasks(self):
        api = API()
        api.task["body"] += "\nСвязано с PR #4"
        request = api.request
        with patch.object(
            api,
            "request",
            side_effect=lambda path: (
                {"number": 4, "pull_request": {}} if path == "/issues/4" else request(path)
            ),
        ):
            c = self.collect(api)
        self.assertEqual([item["task"]["number"] for item in c["issues"]], [7])

    def test_reactions_and_user_metadata_do_not_invalidate_answer(self):
        api = API(pr=True)
        self.start(api)
        self.save(self.collect(api))
        api.task["reactions"] = {"+1": 1, "total_count": 1}
        api.task["updated_at"] = "2026-10-22T12:00:00Z"
        api.issue["reactions"] = {"heart": 1, "total_count": 1}
        api.linked_comments[0]["reactions"] = {"heart": 1}
        api.comments[0]["reactions"] = {"+1": 1}
        api.comments[0]["user"]["avatar_url"] = "https://example.test/new-avatar.png"
        api.reviews[0]["updated_at"] = "2026-10-22T12:00:00Z"
        api.inline[0]["reactions"] = {"+1": 1}
        github.publish_result(api, api.payload)
        self.assertIn("Ответ готов", api.writes[-1][1]["body"])

    def test_content_identity_and_review_state_changes_still_invalidate_answer(self):
        for change in (
            "title",
            "body",
            "base",
            "comment",
            "linked-comment",
            "review",
            "inline",
            "deletion",
        ):
            with self.subTest(change=change):
                api = API(pr=True)
                self.start(api)
                self.save(self.collect(api))
                if change in ("title", "body"):
                    api.task[change] += " changed"
                elif change == "base":
                    api.pr["base"]["sha"] = "c" * 40
                elif change == "comment":
                    api.comments[0]["body"] += " new requirement"
                elif change == "linked-comment":
                    api.linked_comments[0]["body"] += " new requirement"
                elif change == "review":
                    api.reviews[0]["state"] = "DISMISSED"
                elif change == "inline":
                    api.inline[0]["path"] = "another/file.py"
                else:
                    api.comments.pop(0)
                with self.assertRaisesRegex(ValueError, "changed"):
                    github.publish_result(api, api.payload)
                self.assertEqual(len(api.writes), 1)

    def test_context_bounds_and_request_model_usage_are_enforced(self):
        c = self.collect(API())
        r = result(c)
        policy.validate_result(r, c)
        for key, value in (
            ("source_sha", "b" * 40),
            ("comment_id", 31),
            ("thread_number", 3),
            ("mode", "discuss"),
        ):
            bad = copy.deepcopy(r)
            bad["report"][key] = value
            with self.assertRaises(ValueError):
                policy.validate_result(bad, c)
        for key, value in (
            ("model", "other"),
            ("reasoning", "high"),
            ("source_files", ["../secret"]),
            ("skipped_source_files", ["../secret"]),
            ("usage", {"input_tokens": -1}),
        ):
            with self.assertRaises(ValueError):
                policy.validate_result({**r, key: value}, c)
        c["issue_comments"] = [{"body": "x" * 160000}]
        with self.assertRaisesRegex(ValueError, "limit"):
            policy.validate_context(c)

    def test_start_is_idempotent_and_report_edits_same_comment_with_source_links(self):
        for pr in (False, True):
            with self.subTest(pr=pr):
                api = API(pr=pr)
                self.start(api)
                self.start(api)
                c = self.collect(api)
                self.save(c)
                github.publish_result(api, api.payload)
                self.assertEqual([method for _, _, method in api.writes], [None, "PATCH"])
                path, body, _ = api.writes[-1]
                self.assertEqual(path, "/issues/comments/88")
                self.assertIn(f"/blob/{c['source_sha']}/api/main.py#L2-L4", body["body"])
                self.assertIn("input 100", body["body"])
                self.assertIn("gpt-6-luna", body["body"])

    def test_prepare_invoke_publish_preserves_request_and_artifact_boundaries(self):
        api = API(pr=True)
        self.start(api)
        c = self.collect(api)
        with (
            patch.object(github, "GitHub", return_value=api),
            patch.object(github, "event", return_value=api.payload),
            patch.dict(os.environ, {"GITHUB_OUTPUT": str(Path.cwd() / "output")}),
            patch.object(Path, "read_text", return_value="Trusted policy"),
        ):
            github.prepare()
        request = json.loads(Path("request.json").read_text())
        self.assertEqual(request["context"], c)
        self.assertNotIn("GH_TOKEN", request)
        self.assertIn(".agent/instructions.md", request["policies"])
        with patch.object(github, "invoke_json", return_value=result(c)) as invoke:
            github.invoke()
        invoke.assert_called_once_with("agent-question")
        github.publish_result(api, api.payload)
        self.assertEqual(api.writes[-1][0], "/issues/comments/88")

    def test_failure_cancellation_invalid_and_stale_answers_update_same_status(self):
        for failure in (
            "failure",
            "cancelled",
            "source",
            "comment",
            "discussion",
            "linked",
            "invalid",
        ):
            with self.subTest(failure=failure):
                api = API(pr=True)
                self.start(api)
                c = self.collect(api)
                self.save(c)
                if failure == "source":
                    api.pr["head"]["sha"] = "c" * 40
                elif failure == "comment":
                    api.payload["comment"]["body"] += " changed"
                elif failure == "discussion":
                    api.inline.append({"id": 14, "body": "New correction"})
                elif failure == "linked":
                    api.issue["body"] = "Changed linked requirements"
                elif failure == "invalid":
                    Path("conversation-result.json").write_text("{}")
                with (
                    patch.object(github, "GitHub", return_value=api),
                    patch.object(github, "event", return_value=payload()),
                    patch.dict(
                        os.environ,
                        {
                            "CONVERSATION_JOB_RESULT": failure
                            if failure in ("failure", "cancelled")
                            else "success"
                        },
                    ),
                ):
                    if failure in ("failure", "cancelled"):
                        github.publish()
                    else:
                        with self.assertRaises((ValueError, KeyError)):
                            github.publish()
                self.assertEqual(api.writes[-1][0], "/issues/comments/88")
                self.assertIn("⚠️", api.writes[-1][1]["body"])
                self.assertEqual(len(api.writes), 2)

    def test_question_sources_are_complete_bounded_and_include_discussion_paths(self):
        source = Path("source/api")
        source.mkdir(parents=True)
        (source / "main.py").write_text("value = 1\n")
        c = self.collect(API())
        c["question"] = "Explain"
        c["files"] = []
        c["issue_comments"].append({"body": "See api/main.py"})
        self.assertEqual(
            worker.question_sources(Path.cwd(), c), ({"api/main.py": "value = 1\n"}, [])
        )
        (source / "main.py").write_text("x" * 24001)
        with self.assertRaisesRegex(ValueError, "/discuss"):
            worker.question_sources(Path.cwd(), c)

    def test_large_pr_supplies_first_eight_files_and_discloses_omissions(self):
        api = API(pr=True)
        files = [f"api/file{n}.py" for n in range(9)]
        api.files = [{"filename": path} for path in files]
        api.pr["changed_files"] = len(files)
        Path("source/api").mkdir(parents=True)
        for path in files:
            Path("source", path).write_text("value = 1\n")
        self.start(api)
        c = self.collect(api)
        Path("events.jsonl").write_text("")
        with (
            patch.object(worker, "workspace", return_value=nullcontext(Path.cwd())),
            patch.object(worker, "infer", return_value=result(c)["report"]) as infer,
        ):
            response = worker.run({"context": c})
        self.assertEqual(response["source_files"], files[:8])
        self.assertEqual(response["skipped_source_files"], [])
        self.assertIn('"omitted_changed_files": ["api/file8.py"]', infer.call_args.args[2])
        Path("conversation-result.json").write_text(json.dumps({"context": c, "result": response}))
        github.publish_result(api, api.payload)
        self.assertIn("Не переданы изменённые файлы: `api/file8.py`", api.writes[-1][1]["body"])

        # An explicitly referenced path is still supplied beyond the automatic eight.
        c["issue_comments"].append({"body": "Also inspect api/file8.py"})
        sources, skipped = worker.question_sources(Path.cwd(), c)
        self.assertEqual(list(sources), files)
        self.assertEqual(skipped, [])

    def test_incidental_unsafe_paths_do_not_abort_or_escape_question_sources(self):
        Path("source/api").mkdir(parents=True)
        Path("source/api/main.py").write_text("value = 1\n")
        Path("settings.py").write_text("outside snapshot\n")
        Path("source/.git").mkdir()
        Path("source/.git/config").write_text("private metadata\n")
        c = self.collect(API())
        c["issue_comments"].append({"body": "See ../settings.py and .git/config; use api/main.py"})
        self.assertEqual(
            worker.question_sources(Path.cwd(), c), ({"api/main.py": "value = 1\n"}, [])
        )

    def test_explicit_unsafe_source_paths_remain_rejected(self):
        for key in ("files", "existing_plan"):
            c = self.collect(API())
            c[key] = ["../settings.py"]
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, "Invalid source path"):
                worker.question_sources(Path.cwd(), c)

    def test_binary_sources_are_skipped_and_disclosed_in_published_answer(self):
        api = API(pr=True)
        api.files += [{"filename": "assets/logo.png"}, {"filename": "assets/nul.bin"}]
        api.pr["changed_files"] = len(api.files)
        self.start(api)
        c = self.collect(api)
        Path("source/assets").mkdir(parents=True)
        Path("source/api").mkdir()
        Path("source/api/main.py").write_text("value = 1\n")
        Path("source/assets/logo.png").write_bytes(b"\x89PNG\r\n\x1a\n")
        Path("source/assets/nul.bin").write_bytes(b"data\x00value")
        Path("events.jsonl").write_text("")
        with (
            patch.object(worker, "workspace", return_value=nullcontext(Path.cwd())),
            patch.object(worker, "infer", return_value=result(c)["report"]) as infer,
        ):
            response = worker.run({"context": c})
        self.assertEqual(response["source_files"], ["api/main.py"])
        self.assertEqual(response["skipped_source_files"], ["assets/logo.png", "assets/nul.bin"])
        self.assertIn(
            '"skipped_source_files": ["assets/logo.png", "assets/nul.bin"]', infer.call_args.args[2]
        )
        Path("conversation-result.json").write_text(json.dumps({"context": c, "result": response}))
        github.publish_result(api, api.payload)
        self.assertIn("Не прочитаны (бинарные или не UTF-8)", api.writes[-1][1]["body"])
        self.assertIn("assets/logo.png", api.writes[-1][1]["body"])

    def test_question_worker_selects_fixed_model_and_measures_cli_usage(self):
        c = self.collect(API())
        Path("source").mkdir()
        Path("events.jsonl").write_text(
            json.dumps({"type": "turn.completed", "usage": result(c)["usage"]}) + "\n"
        )
        with (
            patch.object(worker, "workspace", return_value=nullcontext(Path.cwd())),
            patch.object(worker, "infer", return_value=result(c)["report"]) as infer,
        ):
            reply = worker.run({"context": c})
        self.assertEqual(reply["usage"]["output_tokens"], 20)
        self.assertEqual(infer.call_args.kwargs["model"], "gpt-6-luna")
        self.assertTrue(infer.call_args.kwargs["question"])
        self.assertIn("Do not use any tools", infer.call_args.args[2])
        Path("events.jsonl").write_text("")
        self.assertIsNone(worker.measured_usage(Path.cwd()))

    def test_discuss_worker_uses_source_inspection_without_question_tool_override(self):
        api = API()
        api.payload["comment"]["body"] = "/discuss Explain the tradeoffs"
        c = self.collect(api)
        Path("events.jsonl").write_text("")
        with (
            patch.object(worker, "workspace", return_value=nullcontext(Path.cwd())),
            patch.object(worker, "infer", return_value=result(c)["report"]) as infer,
        ):
            response = worker.run({"context": c})
        self.assertFalse(infer.call_args.kwargs["question"])
        self.assertEqual(infer.call_args.kwargs["model"], "gpt-6.1-sol")
        self.assertIsNone(response["usage"])
        self.assertIn("context.json", infer.call_args.args[2])

    def test_question_infer_disables_shell_and_rejects_tool_events_without_tokens(self):
        for tools in (False, True):
            with self.subTest(tools=tools):

                def execute(command, tool_use=tools, **kwargs):
                    self.assertEqual(command[command.index("--model") + 1], "gpt-6-luna")
                    self.assertIn("shell_tool", command)
                    self.assertIn('default_permissions="agent-question"', command)
                    self.assertNotIn("GH_TOKEN", kwargs["env"])
                    Path("report.json").write_text("{}")
                    kwargs["stdout"].write(
                        json.dumps(
                            {
                                "type": "item.completed",
                                "item": {
                                    "type": "command_execution" if tool_use else "agent_message"
                                },
                            }
                        )
                        + "\n"
                    )
                    return SimpleNamespace(returncode=0)

                with patch.object(job.subprocess, "run", side_effect=execute):
                    if tools:
                        with self.assertRaisesRegex(ValueError, "tool"):
                            job.infer(
                                Path.cwd(),
                                {},
                                "Question",
                                model="gpt-6-luna",
                                reasoning="low",
                                question=True,
                            )
                    else:
                        self.assertEqual(
                            job.infer(
                                Path.cwd(),
                                {},
                                "Question",
                                model="gpt-6-luna",
                                reasoning="low",
                                question=True,
                            ),
                            {},
                        )

    def test_fixed_commands_allow_only_matching_conversation_mode(self):
        for mode in ("question", "discuss"):
            self.assertEqual(
                review_dispatch.COMMANDS["agent-" + mode], "conversation/ssh_conversation.py"
            )
        c = self.collect(API())
        with (
            patch.object(worker.os, "geteuid", return_value=1000),
            patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": "agent-discuss"}),
            patch.object(worker.sys, "stdin") as stdin,
            patch.object(worker, "run") as run,
            redirect_stdout(io.StringIO()),
            self.assertRaisesRegex(ValueError, "mismatch"),
        ):
            stdin.buffer = io.BytesIO(json.dumps({"context": c}).encode())
            worker.main()
        run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
