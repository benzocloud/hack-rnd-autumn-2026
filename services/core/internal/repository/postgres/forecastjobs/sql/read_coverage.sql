SELECT kind,from_date::text,through_date::text,reference,data_version,limitations FROM core_history_coverage
WHERE company_id=$1 AND account_id=$2 AND data_version=$3
  AND kind IN ('certified','dataset_fixture_assumption')
  AND from_date <= $4 AND through_date >= $5;
