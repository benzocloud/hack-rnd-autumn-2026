INSERT INTO core_forecast_jobs(company_id,account_id,id,data_version,status,created_at,input)
VALUES($1,$2,$3,$4,'queued',$5,$6::jsonb);
