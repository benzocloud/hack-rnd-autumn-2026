INSERT INTO core_expected_payments(company_id,account_id,id,direction,amount_minor,remaining_minor,currency,
scheduled_date,source_kind,source_system,source_company_id,external_id,imported_at,category_id,counterparty_name,description,status,version,created_at,created_data_version)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,''),nullif($11,''),nullif($12,''),$13,$14,$15,$16,$17,$18,$19,$20);
