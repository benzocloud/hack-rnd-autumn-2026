SELECT id,company_id,email,company_name_snapshot,role,status,version,created_at,expires_at,terminal_at,created_by,
 create_idempotency_key,create_request_sha256,accept_idempotency_key,accept_request_sha256
FROM invitations WHERE created_by=$1 AND company_id=$2 AND create_idempotency_key=$3 FOR UPDATE;
