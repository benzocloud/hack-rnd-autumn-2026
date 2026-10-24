INSERT INTO core_source_bindings(company_id,account_id,source_system,source_company_id) VALUES($1,$2,$3,$4)
ON CONFLICT(company_id,account_id,source_system) DO UPDATE SET source_company_id=excluded.source_company_id
WHERE core_source_bindings.source_company_id=excluded.source_company_id;
