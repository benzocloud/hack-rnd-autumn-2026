INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,source_kind,source_system,source_company_id,external_id,imported_at,counterparty_inn,counterparty_name,purpose,version,created_data_version,import_id)
VALUES %s
ON CONFLICT(company_id,account_id,source_system,source_company_id,external_id) WHERE source_kind='imported' DO NOTHING;
