UPDATE core_forecast_jobs
SET status=$5,result=$6::jsonb,error_code=nullif($7,''),request_id=nullif($8,''),
    completed_at=clock_timestamp(),lease_token=NULL,lease_expires_at=NULL
WHERE company_id=$1 AND account_id=$2 AND id=$3 AND status='running'
  AND lease_token=$4 AND lease_expires_at > clock_timestamp();
