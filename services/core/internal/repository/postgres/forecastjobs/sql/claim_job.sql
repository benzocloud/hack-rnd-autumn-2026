WITH candidate AS (
    SELECT company_id,account_id,id FROM core_forecast_jobs
    WHERE status='queued' OR (status='running' AND lease_expires_at <= clock_timestamp())
    ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE core_forecast_jobs AS job
SET status='running',lease_token=$1,lease_expires_at=clock_timestamp()+interval '60 seconds'
FROM candidate
WHERE job.company_id=candidate.company_id AND job.account_id=candidate.account_id AND job.id=candidate.id
RETURNING job.id,job.company_id,job.account_id,job.data_version,job.status,job.created_at,job.completed_at,
          job.lease_token,job.input,job.result,coalesce(job.error_code,'') AS error_code,coalesce(job.request_id,'') AS request_id;
