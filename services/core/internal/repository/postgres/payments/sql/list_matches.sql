SELECT id,transaction_id,amount_minor,currency,created_at,removed_at FROM core_payment_matches
WHERE company_id=$1 AND account_id=$2 AND payment_id=$3 AND ($4::timestamptz IS NULL OR (created_at,id)>($4,$5)) ORDER BY created_at,id LIMIT $6;
