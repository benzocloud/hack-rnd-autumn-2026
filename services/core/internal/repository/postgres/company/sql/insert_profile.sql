INSERT INTO core_company_profiles(company_id,account_id,name,inn,currency,timezone,version,created_at)
VALUES($1,$2,$3,nullif($4,''),$5,$6,$7,$8);
