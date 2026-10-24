SELECT id,direction,amount_minor,remaining_minor,currency,to_char(scheduled_date,'YYYY-MM-DD'),
source_kind,coalesce(source_system,''),coalesce(source_company_id,''),coalesce(external_id,''),imported_at,
category_id,counterparty_name,description,status,version,created_at,created_data_version
FROM core_expected_payments WHERE company_id=$1 AND account_id=$2 AND
(($3::uuid IS NOT NULL AND id=$3) OR ($3::uuid IS NULL AND source_kind='imported' AND source_system=$4 AND source_company_id=$5 AND external_id=$6))
FOR UPDATE;
