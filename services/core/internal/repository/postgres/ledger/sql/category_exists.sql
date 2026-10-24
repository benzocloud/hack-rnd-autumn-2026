SELECT EXISTS(SELECT 1 FROM core_categories WHERE company_id=$1 AND id=$2);
