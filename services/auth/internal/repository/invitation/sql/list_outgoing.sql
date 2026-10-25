SELECT id,company_id,email,company_name_snapshot,role,status,version,created_at,expires_at,terminal_at,created_by,
 create_idempotency_key,create_request_sha256,accept_idempotency_key,accept_request_sha256
FROM invitations
WHERE company_id=$1
 AND (terminal_at IS NULL OR terminal_at > $2)
 AND ($3::timestamptz IS NULL OR created_at < $3 OR (created_at = $3 AND id < $4))
ORDER BY created_at DESC, id DESC
LIMIT $5;
