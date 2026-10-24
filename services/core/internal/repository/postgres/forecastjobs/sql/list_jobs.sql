-- Summaries do not need potentially large completed financial results.
SELECT id,company_id,account_id,data_version,status,created_at,completed_at,lease_token,input,NULL::jsonb AS result,
       coalesce(error_code,'') AS error_code,coalesce(request_id,'') AS request_id
FROM core_forecast_jobs
WHERE company_id=$1 AND account_id=$2
  AND ($3::timestamptz IS NULL OR (created_at,id) < ($3,$4::uuid))
ORDER BY created_at DESC,id DESC LIMIT $5;
