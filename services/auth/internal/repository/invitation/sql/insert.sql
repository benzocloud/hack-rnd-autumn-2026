INSERT INTO invitations(
 id,company_id,email,company_name_snapshot,role,status,version,created_at,expires_at,terminal_at,created_by,
 create_idempotency_key,create_request_sha256,accept_idempotency_key,accept_request_sha256
) VALUES(
 $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
);
