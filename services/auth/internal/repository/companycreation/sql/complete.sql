UPDATE company_creation_commands
SET state='COMPLETED',account_id=$2,committed_at=$3,updated_at=clock_timestamp(),lease_token=NULL,lease_expires_at=NULL
WHERE id=$1 AND state='PREPARED'
AND ($4::uuid IS NULL OR (lease_token=$4 AND lease_expires_at > clock_timestamp()));
