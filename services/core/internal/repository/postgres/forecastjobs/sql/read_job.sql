SELECT id,company_id,account_id,data_version,status,created_at,completed_at,lease_token,input,result,
       coalesce(error_code,'') AS error_code,coalesce(request_id,'') AS request_id
FROM core_forecast_jobs WHERE company_id=$1 AND account_id=$2 AND id=$3;
