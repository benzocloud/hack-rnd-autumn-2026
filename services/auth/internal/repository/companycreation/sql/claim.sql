WITH candidate AS (
 SELECT id FROM company_creation_commands
 WHERE state='PREPARED' AND next_attempt_at <= clock_timestamp()
 AND (lease_token IS NULL OR lease_expires_at <= clock_timestamp())
 ORDER BY next_attempt_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE company_creation_commands AS command
SET lease_token=$1,lease_expires_at=clock_timestamp()+interval '60 seconds',attempts=attempts+1,updated_at=clock_timestamp()
FROM candidate WHERE command.id=candidate.id
RETURNING command.id,command.company_id,command.initiator_id,command.idempotency_key,command.name,command.inn,command.currency,command.timezone,command.request_sha256,command.state,command.created_at,command.updated_at,command.next_attempt_at,command.attempts,command.lease_token,command.lease_expires_at,command.account_id,command.committed_at;
