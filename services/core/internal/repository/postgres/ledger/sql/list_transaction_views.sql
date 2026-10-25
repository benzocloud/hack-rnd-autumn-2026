-- Imported category projection v1. Explicit manual categories take precedence.
-- This read never updates bank facts, their versions, or frozen ML inputs.
SELECT t.id,to_char(t.operation_date,'YYYY-MM-DD'),t.direction,t.amount_minor,t.currency,
       t.source_kind,coalesce(t.source_system,''),coalesce(t.source_company_id,''),
       coalesce(t.external_id,''),t.imported_at,coalesce(t.category_id,automatic.id),
       t.counterparty_inn,t.counterparty_name,t.purpose,t.version,t.created_at,t.import_id
FROM core_transactions t
LEFT JOIN core_categories automatic
  ON automatic.company_id=t.company_id AND t.source_kind='imported' AND t.category_id IS NULL
 AND (automatic.direction IS NULL OR automatic.direction=t.direction)
 AND automatic.catalogue_code=CASE
   WHEN lower(t.purpose) LIKE '%пополнение счета собственными средствами%' THEN 'other'
   WHEN lower(t.purpose) ~ 'перевод собственных средств|между.*счет' THEN 'own_transfer'
   WHEN lower(t.purpose) ~ 'займ|заем|кредит|лизинг|уставн.*капитал|взнос.*капитал|дивиденд' THEN 'financing'
   WHEN lower(t.purpose) ~ '^[[:space:]]*возврат' THEN 'other'
   WHEN t.direction='outflow' AND lower(t.purpose) ~ 'единый налог|\mенп\M|страхов.*взнос|уплата.*налог|перечисление.*налог' THEN 'tax'
   WHEN t.direction='inflow' AND lower(t.purpose) ~ 'оплат|аванс|выручк|эквайринг|сбп|qr|реестр.*продаж|маркетплейс|поступлен' THEN 'sales'
   WHEN t.direction='outflow' AND lower(t.purpose) ~ 'аренд|комисси|ведение счета|обслуживание счета|заработн|зарплат|электроэнерг|водоснаб|коммуналь|отоплен|вывоз|подписк|услуг|связи|интернет|абонентск|оплат|закуп|пополнен|овердрафт' THEN 'operating'
   ELSE 'other'
 END
WHERE t.company_id=$1 AND t.account_id=$2
  AND ($3::date IS NULL OR t.operation_date >= $3)
  AND ($4::date IS NULL OR t.operation_date <= $4)
  AND ($5::text='' OR t.direction=$5)
  AND ($6::uuid IS NULL OR coalesce(t.category_id,automatic.id)=$6)
  AND ($7::date IS NULL OR (t.operation_date,t.id)<($7,$8))
ORDER BY t.operation_date DESC,t.id DESC LIMIT $9;
