INSERT INTO core_transactions(company_id,account_id,id,operation_date,direction,amount_minor,currency,
source_kind,source_system,source_company_id,external_id,imported_at,category_id,counterparty_inn,counterparty_name,purpose,version,created_data_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,nullif($9,''),nullif($10,''),nullif($11,''),$12,$13,$14,$15,$16,$17,$18);
