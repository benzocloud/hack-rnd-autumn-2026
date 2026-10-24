UPDATE core_expected_payments SET remaining_minor=$4,status=$5,version=$6
WHERE company_id=$1 AND account_id=$2 AND id=$3 AND version=$7;
