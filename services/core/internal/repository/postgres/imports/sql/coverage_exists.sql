SELECT EXISTS(SELECT 1 FROM core_history_coverage WHERE company_id=$1 AND account_id=$2 AND data_version=$3 AND kind IN ('certified','dataset_fixture_assumption'));
