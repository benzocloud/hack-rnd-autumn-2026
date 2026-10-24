SELECT balance_date::text AS balance_date,amount_minor,currency,source_kind,
       coalesce(source_system,'') AS source_system,
       coalesce(source_company_id,'') AS source_company_id,recorded_at,created_data_version
FROM core_balances
WHERE company_id=$1 AND account_id=$2 AND balance_date<=$3
ORDER BY balance_date DESC
LIMIT 1;
