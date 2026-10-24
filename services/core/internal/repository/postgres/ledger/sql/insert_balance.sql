INSERT INTO core_balances(company_id,account_id,balance_date,amount_minor,currency,source_kind,source_system,source_company_id,recorded_at,note)
VALUES($1,$2,$3,$4,$5,$6,nullif($7,''),nullif($8,''),$9,$10);
