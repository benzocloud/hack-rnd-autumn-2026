SELECT created_at FROM core_transactions
WHERE company_id=$1 AND account_id=$2 AND id=$3;
