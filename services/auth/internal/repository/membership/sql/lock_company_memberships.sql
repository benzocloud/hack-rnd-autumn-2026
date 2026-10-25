SELECT company_id,user_id,role,created_at FROM memberships WHERE company_id=$1 ORDER BY user_id FOR UPDATE;
