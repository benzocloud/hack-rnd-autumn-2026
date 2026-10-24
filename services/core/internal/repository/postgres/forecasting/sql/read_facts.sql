SELECT id,to_char(operation_date,'YYYY-MM-DD'),direction,amount_minor,currency,
source_kind,coalesce(source_system,''),coalesce(source_company_id,''),coalesce(external_id,''),imported_at,
category_id,counterparty_inn,counterparty_name,purpose,version,created_data_version
FROM core_transactions
WHERE company_id=$1 AND account_id=$2 AND operation_date <= $3
ORDER BY operation_date,id;
