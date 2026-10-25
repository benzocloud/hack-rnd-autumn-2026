WITH staged AS (SELECT DISTINCT ON(external_id) * FROM core_import_rows WHERE import_id=$1 ORDER BY external_id,row_number), counts AS (
SELECT count(*) FILTER(WHERE t.id IS NULL) AS fresh,count(*) FILTER(WHERE t.id IS NOT NULL) AS present FROM staged s LEFT JOIN core_transactions t ON t.company_id=$2 AND t.account_id=$3 AND t.source_system='dataset_v1' AND t.source_company_id=$4 AND t.external_id=s.external_id),issues AS (
SELECT count(*) FILTER(WHERE payload->>'Severity'='blocking') AS blockers,count(*) FILTER(WHERE payload->>'Severity'='warning') AS warnings FROM core_import_issues WHERE import_id=$1)
SELECT COALESCE((SELECT min(operation_date)::text FROM staged),''),COALESCE((SELECT max(operation_date)::text FROM staged),''),fresh,present,blockers,warnings FROM counts,issues;
