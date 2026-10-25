SELECT to_char(max(operation_date),'YYYY-MM-DD') FROM core_transactions WHERE company_id=$1 AND account_id=$2;
