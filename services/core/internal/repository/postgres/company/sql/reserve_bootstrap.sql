INSERT INTO core_company_bootstraps(command_id,company_id,account_id,request_sha256,name,inn,currency,timezone,committed_at)
VALUES($1,$2,$3,$4,$5,nullif($6,''),$7,$8,$9)
ON CONFLICT (command_id) DO NOTHING;
