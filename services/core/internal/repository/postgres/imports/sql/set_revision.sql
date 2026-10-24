UPDATE core_accounts SET data_version=$3,factual_kind='imported',source_system='dataset_v1',source_company_id=$4 WHERE company_id=$1 AND id=$2;
