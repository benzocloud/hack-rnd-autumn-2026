UPDATE core_transactions SET category_id=$4,version=$5 WHERE company_id=$1 AND account_id=$2 AND id=$3 AND version=$6;
