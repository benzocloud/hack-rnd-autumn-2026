SELECT payload FROM core_imports WHERE company_id=$1 AND account_id=$2 AND status<>'receiving' AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4)) ORDER BY created_at DESC,id DESC LIMIT $5;
