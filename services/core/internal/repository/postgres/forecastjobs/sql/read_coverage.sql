SELECT from_date::text,through_date::text,reference FROM core_history_coverage
WHERE company_id=$1 AND account_id=$2 AND data_version=$3
  AND from_date <= $4 AND through_date >= $5;
