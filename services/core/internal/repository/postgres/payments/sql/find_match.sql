SELECT id,transaction_id,amount_minor,currency,created_at FROM core_payment_matches WHERE company_id=$1 AND account_id=$2 AND payment_id=$3 AND id=$4 AND removed_at IS NULL FOR UPDATE;
