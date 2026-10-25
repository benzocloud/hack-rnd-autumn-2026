UPDATE company_creation_commands
SET lease_token=NULL,lease_expires_at=NULL,next_attempt_at=clock_timestamp()+$3 * interval '1 microsecond',updated_at=clock_timestamp()
WHERE id=$1 AND state='PREPARED' AND lease_token=$2 AND lease_expires_at > clock_timestamp();
