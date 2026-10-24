SELECT u.id,u.email,u.display_name,u.state,u.email_verified,u.credential_version,
u.email_verification_method,u.verification_failures,u.verification_window_started_at,
COALESCE(c.password_hash,'') AS password_hash
FROM users u LEFT JOIN credentials c ON c.user_id=u.id
WHERE u.id=$1;
