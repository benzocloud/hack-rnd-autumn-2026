INSERT INTO core_command_results(actor_id,company_id,account_id,operation,resource_id,idempotency_key,digest)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING;
