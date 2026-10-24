SELECT data_version,to_char(cutoff,'YYYY-MM-DD'),created_at,payload_version,payload
FROM core_financial_snapshots WHERE company_id=$1 AND account_id=$2 AND id=$3;
