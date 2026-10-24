UPDATE verification_challenges SET failed_attempts=$2,consumed_at=$3 WHERE id=$1;
