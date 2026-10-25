SELECT payload FROM core_imports WHERE company_id=$1 AND account_id=$2 AND id=$3 FOR UPDATE;
