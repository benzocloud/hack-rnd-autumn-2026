SELECT command_id,company_id,account_id,request_sha256,name,coalesce(inn,'') AS inn,currency,timezone,committed_at
FROM core_company_bootstraps WHERE command_id=$1;
