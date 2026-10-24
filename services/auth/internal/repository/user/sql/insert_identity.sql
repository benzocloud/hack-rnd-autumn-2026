INSERT INTO users(id,email,display_name) VALUES($1,$2,$3) ON CONFLICT(email) DO NOTHING RETURNING id;
