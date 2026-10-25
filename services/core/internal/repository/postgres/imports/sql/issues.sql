SELECT id,payload FROM core_import_issues WHERE import_id=$1 AND id>$2 ORDER BY id LIMIT $3;
