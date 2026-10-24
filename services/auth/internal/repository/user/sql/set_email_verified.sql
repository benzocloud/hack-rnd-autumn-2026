UPDATE users SET email_verified=true,email_verification_method=$2 WHERE id=$1;
