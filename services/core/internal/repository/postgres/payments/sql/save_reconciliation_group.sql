UPDATE core_payment_reconciliation_groups SET version=$4,status=$5 WHERE company_id=$1 AND account_id=$2 AND id=$3 AND version=$6;
