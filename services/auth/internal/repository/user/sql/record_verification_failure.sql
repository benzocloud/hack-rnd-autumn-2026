UPDATE users SET
verification_failures=CASE WHEN verification_window_started_at IS NULL OR verification_window_started_at <= $2::timestamptz - interval '15 minutes' THEN 1 ELSE LEAST(5,verification_failures+1) END,
verification_window_started_at=CASE WHEN verification_window_started_at IS NULL OR verification_window_started_at <= $2::timestamptz - interval '15 minutes' THEN $2 ELSE verification_window_started_at END
WHERE id=$1;
