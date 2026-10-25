SELECT id,direction,amount_minor,remaining_minor,currency,to_char(scheduled_date,'YYYY-MM-DD'),
source_kind,coalesce(source_system,''),coalesce(source_company_id,''),coalesce(external_id,''),imported_at,
category_id,counterparty_name,description,status,version,created_at,created_data_version,reconciliation_group_id
FROM core_expected_payments WHERE company_id=$1 AND account_id=$2
AND ($3::date IS NULL OR scheduled_date >= $3) AND ($4::date IS NULL OR scheduled_date <= $4)
AND (($5='' AND status IN ('planned','partially_matched')) OR status=$5)
AND ($6::date IS NULL OR (scheduled_date,id)>($6,$7)) ORDER BY scheduled_date,id LIMIT $8;
