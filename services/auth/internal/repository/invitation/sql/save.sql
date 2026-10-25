UPDATE invitations SET
 status=$2,
 version=$3,
 expires_at=$4,
 terminal_at=$5,
 accept_idempotency_key=$6,
 accept_request_sha256=$7
WHERE id=$1 AND version=$8;
