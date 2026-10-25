SELECT payload FROM core_imports WHERE company_id=$1 AND account_id=$2 AND actor_id=$3 AND idempotency_key=$4 FOR UPDATE;
