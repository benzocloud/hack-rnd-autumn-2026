SELECT EXISTS (
    SELECT 1 FROM core_transactions
    WHERE company_id=$1 AND account_id=$2 AND source_kind='imported'
      AND operation_date=$3 AND direction=$4 AND amount_minor=$5
      AND counterparty_inn=$6 AND counterparty_name=$7 AND purpose=$8
);
