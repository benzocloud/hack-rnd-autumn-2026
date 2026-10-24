SELECT data_version,COALESCE(factual_kind,''),COALESCE(source_system,''),COALESCE(source_company_id,'') FROM core_accounts WHERE company_id=$1 AND id=$2 FOR UPDATE;
