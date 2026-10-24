UPDATE core_company_profiles
SET name=$2, inn=nullif($3,''), version=$4
WHERE company_id=$1 AND version=$5;
