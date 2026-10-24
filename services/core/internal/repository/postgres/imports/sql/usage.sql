SELECT COALESCE(sum(reserved_bytes) FILTER (WHERE company_id=$1 AND account_id=$2 AND NOT raw_deleted),0),
COALESCE(sum(reserved_bytes) FILTER(WHERE NOT raw_deleted),0),
count(*) FILTER (WHERE company_id=$1 AND account_id=$2 AND status IN ('receiving','queued','processing','preview_ready')) FROM core_imports;
