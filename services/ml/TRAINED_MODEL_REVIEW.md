# Проверки и ревью ML-профиля

25.10.2026. Локальный scope — только services/ml; исходный main
`0f0ff739d633e4fd1ac34e5fb5f125fbbd4c7636`. Два прохода саморевью по
указанным skills; отдельный независимый reviewer этим не утверждается.
Go skills не запускались. Issue не назначен; требования — поручение владельца,
ADR-029, актуальные proto/код, открытые PR #42/#44 и их доступное обсуждение.
Оба PR не имеют опубликованных комментариев на момент проверки.

Прочитаны pinned инструкции из repository registry `.agent/infra/skills/skills.json`:

- [caveman-review](https://github.com/JuliusBrussee/caveman/blob/bad48a0e1df55e05e3f52249afa35ecc1909ee76/skills/caveman-review/SKILL.md),
  commit bad48a0e1df55e05e3f52249afa35ecc1909ee76, blob ed2bf49948c1514c147a699a484456a5ff7b27f7.
- [ponytail-review](https://github.com/DietrichGebert/ponytail/blob/09c434ab73ca43761c5d8b0c5d341926c2fba710/skills/ponytail-review/SKILL.md),
  commit 09c434ab73ca43761c5d8b0c5d341926c2fba710, blob a50e3ac6ded6fccc9edae34ce37877bbce11e3fa.

## Caveman: исправленные подтверждённые замечания

`cashflow_rpc/trained_pool.py:L65`: non-ASCII checksum could escape as UNKNOWN.
Catch UnicodeError as explicit UnsupportedContext; regression covers bad bytes. Fixed.

`cashflow_rpc/model_assets/.gitattributes:L1`: Windows newline conversion could
change the pinned JSON checksum on checkout. Mark the JSON -text. Fixed;
git check-attr confirms text is unset, artifact is hashed from exact bytes.

`cashflow_rpc/financial.py`: the fitted corner needlessly built 30 category
feature maps despite having exactly the old allocation. Reuse that allocation
algebraically, keeping the learned identity; test guards against materializing
those maps. Fixed. This is not fallback after an error.

Открытых подтверждённых дефектов после исправлений не найдено.

## Ponytail

What this change does: Trains a small flow mixture and exposes its pinned
artifact through the existing private Predict. Training chose the mean_90
corner, so no quality gain or product switch is claimed.

Checked: calls from server selection through shared financial validation,
replacement before features, category allocation and exact balances; model
identity, missing/corrupt artifacts, historical availability, data cutoffs,
scope/calendar echoes and probability semantics. Read connected Core checks
and current Docker COPY/ignore paths, not only the Python diff.

Expected load: existing two workers/four concurrent RPC, 2 MiB and 20 000
operation bounds; examples measured on 303/74 operations. Production deploy
has 0.5 CPU/192 MiB. No change to transport or concurrency. No arbitrary
model download, unpickling, DB access or background state writes.

The new profile is deliberately experimental. All learned weights are mean_90;
the full training pipeline is retained for reproducibility and an explicit
versioned inference path, not evidence for improving the site. Core remains
blocked; changing its validators is outside this PR. No automatic old-model
retry is introduced.

Money above float precision, missing context, wrong seed/checksum, invalid
replacement, overflow, new/moved/partial/cancelled payments and UNAVAILABLE
calendar are tested. Cancellation after partial settlement keeps the paid fact
and the lower observed EOD. Old profile responses are byte-identical in five
pre-change scenarios. Both sample companies match offline over real local mTLS.

Verdict: Ship.

This is an advisory verdict for a draft, non-default ML PR. It does not approve
merge/deployment or recommend enabling the new profile in Core.
Not checked: real Go Core invocation (unsupported context), browser E2E,
Docker image build and worst-case concurrent load at the production limit.

## Фактические проверки

- Полный ML unittest: **120 passed, 0 skipped**.
- Полный RPC/mTLS/financial suite: **45 passed, 0 skipped**.
- Ruff 0.17.1 check и format --check: PASS, 61 Python-файл.
- Воспроизведение обучения: artifact, 225 losses, metrics/per-company и все
  прогнозы совпали; SHA 5559b7dd54327bcd0e86a72d15e6073e4d3e73f56225e09ce00b17bb396c63b9.
- Независимый арифметический audit: 21 750 дневных точек frozen baseline,
  MAE/RMSE — PASS. Исходные CSV не менялись.
- mTLS на двух компаниях + synthetic lifecycle + isolated package load — PASS;
  в runtime нет зависимости от dataset или рабочего каталога обучения.
- git diff --check: PASS. Перед публикацией повторно проверяются fresh main,
  ancestor, индекс/разрешённые пути и отсутствие dataset/bindings/ключей.

Generated bindings локально получены grpc_tools из текущего canonical proto,
как в существующей Docker-сборке; они не публикуются. Это не утверждение,
что локально запускался Buf: proto не менялся, Buf/drift отдельно проверяет CI.
GitHub CI и head SHA указываются после публикации в PR/итоговом ответе;
локальные PASS не выдаются за уже выполненный CI.
