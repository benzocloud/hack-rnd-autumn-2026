INSERT INTO core_import_issues(import_id,payload)
SELECT $1,jsonb_build_object('Code',code,'Severity',severity,'Row',row_number,'Field',field,'ExternalIDs',ids) FROM (
SELECT 'CONFLICTING_EXTERNAL_ID' AS code,'blocking' AS severity,min(row_number) AS row_number,'operation_id' AS field,jsonb_build_array(external_id) AS ids FROM core_import_rows WHERE import_id=$1 GROUP BY external_id HAVING count(DISTINCT economy)>1
UNION ALL
SELECT 'SUSPECTED_CONTENT_DUPLICATE','warning',min(s.row_number),'operation_id',jsonb_build_array(s.external_id)
FROM core_import_rows s JOIN (SELECT content_hash FROM core_import_rows WHERE import_id=$1 GROUP BY content_hash HAVING count(DISTINCT external_id)>1) duplicates ON duplicates.content_hash=s.content_hash
WHERE s.import_id=$1 GROUP BY s.external_id
UNION ALL
SELECT 'CONFLICTING_EXTERNAL_ID','blocking',s.row_number,'operation_id',jsonb_build_array(s.external_id)
FROM core_import_rows s JOIN core_transactions t ON t.company_id=$2 AND t.account_id=$3 AND t.source_system='dataset_v1' AND t.source_company_id=$4 AND t.external_id=s.external_id
WHERE s.import_id=$1 AND (s.operation_date<>t.operation_date OR s.amount_minor<>t.amount_minor OR s.direction<>t.direction OR s.payload->>'INN'<>t.counterparty_inn OR s.payload->>'Name'<>t.counterparty_name OR s.payload->>'Purpose'<>t.purpose)
UNION ALL
SELECT 'SUSPECTED_CONTENT_DUPLICATE','warning',s.row_number,'manual_transaction',jsonb_build_array(s.external_id)
FROM core_import_rows s
WHERE s.import_id=$1 AND EXISTS (
    SELECT 1 FROM core_transactions t
    WHERE t.company_id=$2 AND t.account_id=$3 AND t.source_kind='manual'
      AND t.operation_date=s.operation_date AND t.direction=s.direction AND t.amount_minor=s.amount_minor
      AND t.counterparty_inn=s.payload->>'INN' AND t.counterparty_name=s.payload->>'Name'
      AND t.purpose=s.payload->>'Purpose'
)
UNION ALL
SELECT 'INVALID_ROW','blocking',min(row_number),'balance_eod','[]'::jsonb FROM core_import_rows WHERE import_id=$1 GROUP BY operation_date HAVING count(DISTINCT balance_minor)>1
UNION ALL
SELECT 'INVALID_ROW','blocking',min(s.row_number),'balance_eod','[]'::jsonb FROM core_import_rows s JOIN core_balances b ON b.company_id=$2 AND b.account_id=$3 AND b.balance_date=s.operation_date WHERE s.import_id=$1 AND s.balance_minor<>b.amount_minor GROUP BY s.operation_date
UNION ALL
SELECT 'SOURCE_BINDING_CONFLICT','blocking',0,'source_company_id','[]'::jsonb FROM core_source_bindings WHERE company_id=$2 AND account_id=$3 AND source_system='dataset_v1' AND source_company_id<>$4
UNION ALL
SELECT 'BALANCE_DISCONTINUITY','warning',row_number,'balance_eod','[]'::jsonb FROM (
SELECT min(row_number) AS row_number,min(balance_minor)::numeric AS balance,lag(min(balance_minor)::numeric) OVER(ORDER BY operation_date) AS previous_balance,
sum(CASE direction WHEN 'inflow' THEN amount_minor::numeric ELSE -amount_minor::numeric END) AS net FROM (SELECT DISTINCT ON(external_id) * FROM core_import_rows WHERE import_id=$1 ORDER BY external_id,row_number) unique_rows GROUP BY operation_date) daily
WHERE previous_balance IS NOT NULL AND abs(balance-previous_balance-net)>1
) diagnostics;
