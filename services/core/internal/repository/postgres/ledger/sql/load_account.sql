SELECT data_version,coalesce(factual_kind,''),coalesce(source_system,''),coalesce(source_company_id,'')
FROM core_accounts WHERE company_id=$1 AND id=$2 FOR UPDATE;
