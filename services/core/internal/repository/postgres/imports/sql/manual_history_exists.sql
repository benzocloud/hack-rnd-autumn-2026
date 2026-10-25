SELECT EXISTS (
    SELECT 1 FROM core_transactions
    WHERE company_id=$1 AND account_id=$2 AND source_kind='manual'
      AND operation_date BETWEEN $3::date AND $4::date
) OR EXISTS (
    SELECT 1 FROM core_balances
    WHERE company_id=$1 AND account_id=$2 AND source_kind='manual'
      AND balance_date BETWEEN $3::date AND $4::date
);
