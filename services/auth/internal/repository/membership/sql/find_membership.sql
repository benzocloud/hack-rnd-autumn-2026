SELECT company_id,user_id,role,created_at FROM memberships WHERE company_id=$1 AND user_id=$2;
