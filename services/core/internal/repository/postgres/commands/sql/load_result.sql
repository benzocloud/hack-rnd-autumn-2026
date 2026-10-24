SELECT digest,result FROM core_command_results
WHERE actor_id=$1 AND company_id=$2 AND account_id=$3 AND operation=$4 AND resource_id=$5 AND idempotency_key=$6
FOR UPDATE;
