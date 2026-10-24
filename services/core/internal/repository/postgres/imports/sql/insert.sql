INSERT INTO core_imports(id,company_id,account_id,actor_id,idempotency_key,status,reserved_bytes,created_at,lease_until,token,payload)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);
