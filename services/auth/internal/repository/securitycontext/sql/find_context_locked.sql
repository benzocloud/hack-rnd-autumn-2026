SELECT c.id,c.kind,c.token_hash,c.csrf_nonce,c.csrf_key_id,c.created_at,c.expires_at,c.revoked_at,
COALESCE(s.user_id,'00000000-0000-0000-0000-000000000000'::uuid) AS user_id,
COALESCE(s.credential_version,0) AS credential_version,COALESCE(s.last_seen_at,c.created_at) AS last_seen_at
FROM security_contexts c LEFT JOIN sessions s ON s.context_id=c.id WHERE c.token_hash=$1 FOR UPDATE OF c;
