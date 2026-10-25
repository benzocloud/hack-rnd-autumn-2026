UPDATE core_payment_matches SET removed_at=clock_timestamp() WHERE company_id=$1 AND account_id=$2 AND id=$3 AND removed_at IS NULL;
