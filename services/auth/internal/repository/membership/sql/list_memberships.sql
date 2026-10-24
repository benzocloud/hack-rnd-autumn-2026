SELECT company_id,user_id,role,created_at FROM memberships
WHERE user_id=$1 AND ($2::timestamptz IS NULL OR (created_at,company_id)<($2,$3))
ORDER BY created_at DESC,company_id DESC LIMIT $4;
