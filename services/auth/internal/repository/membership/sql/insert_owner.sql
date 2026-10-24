INSERT INTO memberships(company_id,user_id,role,created_at) VALUES($1,$2,'owner',$3)
ON CONFLICT(company_id,user_id) DO UPDATE SET role=memberships.role WHERE memberships.role='owner';
