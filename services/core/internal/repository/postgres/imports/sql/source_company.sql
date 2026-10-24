SELECT source_company_id FROM core_source_bindings WHERE company_id=$1 AND account_id=$2 AND source_system='dataset_v1';
