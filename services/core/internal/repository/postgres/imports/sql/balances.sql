SELECT operation_date::text,min(balance_minor) FROM core_import_rows WHERE import_id=$1 GROUP BY operation_date ORDER BY operation_date LIMIT 1001;
