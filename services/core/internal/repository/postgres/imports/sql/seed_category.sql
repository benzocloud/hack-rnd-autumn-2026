INSERT INTO core_categories(company_id,id,name,direction,semantic_kind,catalogue_code)
VALUES($1,$2,$3,NULLIF($4,''),$5,$6)
ON CONFLICT(company_id,catalogue_code) WHERE catalogue_code IS NOT NULL DO NOTHING;
