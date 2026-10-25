# Матрица сценариев финансовой приёмки

Колонки статуса в отчёте: `implemented` / `automated contract` /
`real HTTP` / `real browser` / `restart` / `blocked`.

Легенда ячеек harness: `todo` | `skip` | `pass` | `fail` | `blocked:<reason>`.

| ID | Сценарий | Priority | Expected (контракт) | HTTP | Browser | Restart |
| --- | --- | --- | --- | --- | --- | --- |
| P0-01 | register → consent version/digest | P0 | account pending verify; no auto-login | todo | todo | — |
| P0-02 | verify-email demo allowlist | P0 | demo profile only; strict rejects bad code | todo | todo | — |
| P0-03 | login + CSRF refresh | P0 | session cookie; financial needs verified | todo | todo | — |
| P0-04 | create company (or staff test company) | P0 | owner membership durable; no SQL facts | todo | todo | — |
| P0-05 | upload ordinary 90d CSV | P0 | 202 + durable job; no ledger yet | todo | todo | — |
| P0-06 | preview / issues / ack | P0 | can_commit after required acks | todo | todo | — |
| P0-07 | commit import | P0 | facts+EOD+source; CAS | todo | todo | — |
| P0-08 | list ledger / account | P0 | counts via public API; no naive CSV sum = balance | todo | todo | — |
| P0-09 | create forecast | P0 | 202; poll succeeded; 30 points | todo | todo | — |
| P0-10 | reload saved forecast | P0 | same result; immutable bytes | todo | todo | todo |
| R-01 | re-upload same statement | P1 | no duplicate facts | todo | skip | — |
| R-02 | replay same idempotency key | P1 | equivalent replay | todo | skip | — |
| R-03 | manual op after import | P1 | allowed; manual provenance; namespace unchanged | todo | todo | — |
| R-04 | replay manual | P1 | once | todo | skip | — |
| R-05 | exact imported duplicate as manual | P1 | rejected | todo | skip | — |
| R-06 | next CSV matches manual | P1 | requires explicit ack; both kept; no auto-merge | todo | todo | — |
| R-07 | after reload/restart facts persist | P1 | public API counts | todo | todo | todo |
| R-08 | old forecast stale after factual change | P1 | not rewritten | todo | todo | — |
| PAY-01 | payment create → forecast | P1 | new points use revision; needs #48 | todo | todo | — |
| PAY-02 | amount edit → date edit | P1 | new forecast points change by mode | todo | todo | — |
| PAY-03 | partial match → unmatch → partial | P1 | match links fact; no second fact | todo | todo | — |
| PAY-04 | cancel → history → new forecast | P1 | remaining cancelled; history readable | todo | todo | — |
| PAY-05 | forbidden transition | P1 | explicit rejection | todo | skip | — |
| PAY-06 | covered fixture vs ordinary CSV | P1 | separate; fixture ≠ CSV calendar proof | todo | skip | — |
| PAY-07 | complete replacement residual | P1 | no historic residual without revoke | todo | skip | — |
| N-01 | invalid / partial upload | P2 | no accepted corrupt job | todo | skip | — |
| N-02 | duplicate key different body | P2 | conflict | todo | skip | — |
| N-03 | stale preview / CAS | P2 | explicit stale | todo | skip | — |
| N-04 | short history | P2 | reject / limitation; not zero risk | todo | skip | — |
| N-05 | EOD conflict | P2 | block commit | todo | skip | — |
| N-06 | missing cutoff anchor | P2 | reject | todo | skip | — |
| N-07 | session expiry / revocation | P2 | 401/403; no financial leak | todo | skip | — |
| N-08 | CSRF missing/invalid | P2 | 403 CSRF_INVALID | todo | skip | — |
| N-09 | foreign company/resource/cursor | P2 | denied | todo | skip | — |
| N-10 | ML unavailable | P2 | explicit failure; not zero probability | todo | skip | — |
| N-11 | unknown days / missing probability | P2 | no zero-fill; no fabricated 0% | todo | todo | — |

Обязательные P0 со статусом `skip` не делают общий результат зелёным.
