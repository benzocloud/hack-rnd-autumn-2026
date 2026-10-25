SELECT m.company_id,m.user_id,m.role,m.created_at,u.email,u.display_name
FROM memberships m
JOIN users u ON u.id=m.user_id
WHERE m.company_id=$1
 AND ($2::timestamptz IS NULL OR m.created_at < $2 OR (m.created_at = $2 AND m.user_id < $3))
ORDER BY m.created_at DESC, m.user_id DESC
LIMIT $4;
