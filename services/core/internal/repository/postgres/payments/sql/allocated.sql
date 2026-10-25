SELECT coalesce(sum(amount_minor),0)::text FROM core_payment_matches
WHERE company_id=$1 AND account_id=$2 AND transaction_id=$3 AND removed_at IS NULL;
