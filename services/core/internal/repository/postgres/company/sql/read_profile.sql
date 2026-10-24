SELECT company_id,account_id,name,coalesce(inn,'') AS inn,currency,timezone,version,created_at
FROM core_company_profiles WHERE company_id=$1;
