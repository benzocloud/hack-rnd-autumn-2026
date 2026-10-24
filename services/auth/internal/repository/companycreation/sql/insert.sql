INSERT INTO company_creation_commands(id,company_id,initiator_id,idempotency_key,name,inn,currency,timezone,request_sha256,state,created_at,updated_at,next_attempt_at)
VALUES($1,$2,$3,$4,$5,nullif($6,''),$7,$8,$9,'PREPARED',$10,$10,$10);
