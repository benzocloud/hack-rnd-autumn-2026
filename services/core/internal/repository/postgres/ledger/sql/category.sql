SELECT name,COALESCE(direction,''),semantic_kind,COALESCE(catalogue_code,'') FROM core_categories WHERE company_id=$1 AND id=$2;
