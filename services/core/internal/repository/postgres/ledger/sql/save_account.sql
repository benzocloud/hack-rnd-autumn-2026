UPDATE core_accounts SET data_version=$3,factual_kind=nullif($4,''),source_system=nullif($5,''),source_company_id=nullif($6,'')
WHERE company_id=$1 AND id=$2;
