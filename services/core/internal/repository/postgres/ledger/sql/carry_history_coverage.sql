INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference,
kind,origin_data_version,artifact_sha256,import_id,limitations)
SELECT company_id,account_id,$4,from_date,through_date,reference,
kind,origin_data_version,artifact_sha256,import_id,limitations
FROM core_history_coverage WHERE company_id=$1 AND account_id=$2 AND data_version=$3;
