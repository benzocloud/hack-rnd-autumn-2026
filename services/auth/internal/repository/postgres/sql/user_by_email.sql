SELECT id, email, display_name, email_verified, credential_version, state, created_at
FROM users WHERE email = $1;
