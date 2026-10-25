# Saldo runner pool

The private repository uses two repository-level Linux x64 runners:

| Host | Runner name | Labels | Shared CPU / RAM budget |
| --- | --- | --- | --- |
| gbrain.brokebyte.me | saldo-gbrain | saldo-ci, saldo-build, gbrain | 150% CPU, 2800 MiB |
| runner.brokebyte.me | saldo-runner | saldo-ci, runner | 100% CPU, 1500 MiB |

Jobs with `[self-hosted, Linux, X64, saldo-ci]` run on the next available
matching runner. Each host accepts one job at a time. Image builds use the
additional `saldo-build` label because the GBrain host has more free memory
and disk. The Fadwix host does not build production images.

Both runners execute as `saldo-ci` from `/opt/saldo-actions`, with no sudo or
host Docker group membership. Docker is a separate rootless user service.
Its socket is `/run/user/<uid>/docker.sock`. CPU/RAM limits cover the runner
and its rootless daemon together through `user-<uid>.slice`. Existing GBrain,
Fadwix services, rootful Docker, containers and credentials remain separate.
The GBrain host has a dedicated AppArmor user-namespace allowance for
`/home/saldo-ci/bin/rootlesskit`; no global AppArmor restriction is disabled.

Runtime dependencies (compiler, Python venv, protoc, psql, jq and shellcheck)
are installed by the administrator. Jobs do not run apt or receive sudo.
GitHub setup actions install Go/Node into the writable runner tool cache.
External fork PR jobs are rejected before assignment to these runners.

## Faster delivery

Full product CI runs on PRs or manual dispatch. Main runs the integrated
delivery pipeline instead of repeating every module suite and image publish.
The frontend workflow validates the real frontend without building or deploying
the former mock image. Core CI builds/tests Go packages without publishing a
second runtime image. Full race/lint/coverage/vulnerability checks remain in PR
CI; the deployment preflight runs short Go unit tests, frontend tests/typecheck
and deployment-script checks. Merge accepted PRs only after their full gates.

The integrated workflow builds the five production images serially using a
persistent `saldo-stack` BuildKit cache. Go module/compiler and Next.js caches
are reused. Digests pass through job outputs, without Actions artifact storage.
The existing restricted SSH deployment, immutable digest validation, migration
backup, rollback and public financial smoke remain in place. No mock workflow
deploys over the integrated site.

Run **Self-hosted runner smoke** to test both hosts. An online runner alone
does not prove that account billing permits a job or that a release deploys.
Check the workflow result before claiming delivery restored. The first build
starts with cold caches and will take longer than later builds.

## Operations

Runner service names:

```
actions.runner.benzocloud-hack-rnd-autumn-2026.saldo-gbrain.service
actions.runner.benzocloud-hack-rnd-autumn-2026.saldo-runner.service
```

Inspect these services and the `saldo-ci` user's Docker service only. Do not
restart the shared rootful Docker daemon, prune its storage, alter other
containers or remove their volumes. Monitor CI-user disk usage: persistent
tool/BuildKit caches are local and remain subject to available disk capacity.
Registration credentials are stored under the runner's private directory and
must not enter Git or logs. Removing the pool requires disabling its runner
services and removing the matching repository registrations; workflow changes
alone do not remove host state.
