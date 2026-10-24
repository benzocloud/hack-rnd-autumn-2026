SELECT id,name,COALESCE(direction,''),semantic_kind,COALESCE(catalogue_code,'') FROM core_categories WHERE company_id=$1 AND ($2::uuid IS NULL OR id>$2) ORDER BY id LIMIT $3;
