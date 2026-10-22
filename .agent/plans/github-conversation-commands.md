# GitHub questions and discussion

Approved implementation scope, 2026-10-22.

## Behavior

- `/question <question>` in an open Issue or same-repository PR produces a
  concise read-only answer using the full available discussion and relevant
  source context. Use `gpt-6-luna`, low reasoning, with execution tools disabled.
- `/discuss <message>` uses `gpt-6.1-sol`, low reasoning, for source-aware
  discussion. In ordinary Issues/PRs it only replies. In a managed plan-only PR,
  an explicit request to amend the plan routes through the existing fixed-file
  planning publisher. Questions about how to amend a plan do not authorize edits.
- `/discuss-plan` is removed without an alias. `/create-plan` and `/agent-review`
  keep their existing behavior. Internal planning protocol mode names are not
  public slash commands and need not be renamed during this rollout.
- Both new commands require a newly created human comment and repository
  write/maintain/admin permission. Edited comments and workflow reruns do not
  trigger inference. Replies use one progress comment per request, edited with
  the final answer or failure. Source links stay bound to the assessed SHA.

## Context and authority

Read the description and all available conversation, review and inline comments,
linked Issues, current plan when present, source SHA and relevant source files.
Fail visibly on incomplete pagination or exceeded input bounds. Treat discussion
and snapshot instructions as untrusted task data. Reject stale source/task or
changed command comments before publication. Model output never selects write
paths or receives GitHub credentials. Conversation publishers cannot write code.

Keep ChatGPT Plus authentication. `/question` is a lighter Codex execution mode,
not a separate ChatGPT Chat allowance: official ChatGPT plan integrations use
Codex/ChatGPT Work limits. Record actual token usage when the CLI provides it;
do not promise zero usage or a particular saving before measurement.

## Delivery

- [x] Extend command parsing and reject the removed slash command.
- [x] Add bounded context collection and a read-only conversation worker/publisher.
- [x] Add tool-disabled question execution and explicit model/effort selection.
- [x] Wire GitHub permissions, fixed SSH commands and installer paths.
- [x] Update instructions and operator documentation.
- [x] Verify parser, authorization, context, fixed plan destination, stale/failure
  handling, tool restrictions and workflows with focused tests and linters.

Use `.agent/infra/conversation/` for conversation orchestration, reuse shared
GitHub transport and isolated runner lifecycle, and reuse the planning publisher
for amendments. No API billing setup, application code changes, deployment,
Fadwix changes or automatic merge are included. Existing clickable-reference
changes remain intact. Publication and runner rollout require the usual approval.

Local verification: 57 stdlib tests passed, including the prior review/planning
regressions, conversation request/artifact/publication flow, managed-plan status
updates, current-command routing and question tool-event rejection. Ruff lint,
format checks, actionlint and `git diff --check` passed. At this local verification
point, workflow/adapters/profile changes had not been deployed.

Subsequent rollout, 2026-10-22: after explicit owner approval, PR #7 merged and
the matching runner adapters/profile were installed under the review lock with
a private backup. Both selected models passed dispatcher fixtures. Live Issue #8
and PR #9 verified questions, plan creation/amendment, ordinary PR discussion
with linked Issue context, and explicit review, including editing one status
comment per request. Both objects were closed manually without implementation or
merge. The measured limits and rollout provenance are in `.agent/infra/README.md`;
in particular, encoded plan state can exceed the question-source size limit.

Review fixes: binary/non-UTF-8 question sources are skipped with explicit unread
receipts, ordinary PR references are not mistaken for Issue tasks, and freshness
checks compare substantive task/discussion fields instead of reactions or API
display metadata. Regression tests also retain rejection of changed source/base,
requirements, comments, inline locations and review state.

Owner clarification: generated plan commits and draft PR titles use `feat:`,
the Issue number and a short English fix description. Commit bodies identify
the requester and command and provide a direct `Request:` comment permalink;
the same exact line prevents duplicate plan commits. Draft PR descriptions also
link to that comment. Publication tests cover both Issue/PR comment links,
English subjects/bodies and exact-link duplicate detection.

## Follow-up review fixes

Status: accepted by the developer on 2026-10-22 after local verification.

- Plan edits require a leading action directed at the plan itself; incidental
  mentions and questions remain read-only discussion.
- Questions receive the first eight changed files even for larger PRs. Other
  changed files not supplied are disclosed to the model and in the final reply;
  explicitly referenced safe paths can still be supplied within size limits.
- Unsafe paths extracted from discussion are ignored. Explicit source/plan path
  lists retain strict validation.

Verified: 20 conversation tests and all 73 infrastructure tests, Ruff lint/format
and `git diff --check` passed. These tests use model/GitHub mocks and do not
consume model tokens. Developer approval accepts these fixes; no commit, push,
merge or runner deployment was requested.

The developer subsequently authorized committing all remaining local changes
on 2026-10-22. Push, merge and runner deployment remain separate steps.

References: [ChatGPT plan usage](https://learn.chatgpt.com/docs/sign-in-with-chatgpt#use-your-chatgpt-plan),
[Codex configuration](https://learn.chatgpt.com/docs/config-file/config-reference).
