UPDATE verification_challenges SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL;
