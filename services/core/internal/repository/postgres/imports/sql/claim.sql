SELECT payload FROM core_imports WHERE status='queued' OR(status='processing' AND lease_until<$1) ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED;
