UPDATE company_creation_commands
SET state='QUARANTINED',updated_at=clock_timestamp(),lease_token=NULL,lease_expires_at=NULL
WHERE id=$1 AND state='PREPARED'
AND ($2::uuid IS NULL OR (lease_token=$2 AND lease_expires_at > clock_timestamp()));
