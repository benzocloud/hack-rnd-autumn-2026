INSERT INTO core_history_coverage(company_id,account_id,data_version,from_date,through_date,reference,kind,origin_data_version,artifact_sha256,import_id,limitations) VALUES($1,$2,$3,$4,$5,$6,$7,$3,$8,$9,$10)
ON CONFLICT(company_id,account_id,data_version) DO UPDATE SET
  from_date=EXCLUDED.from_date, through_date=EXCLUDED.through_date, reference=EXCLUDED.reference,
  kind=EXCLUDED.kind, origin_data_version=EXCLUDED.origin_data_version,
  artifact_sha256=EXCLUDED.artifact_sha256, import_id=EXCLUDED.import_id, limitations=EXCLUDED.limitations
WHERE (core_history_coverage.kind IS NULL AND core_history_coverage.origin_data_version IS NULL
       AND core_history_coverage.artifact_sha256 IS NULL AND core_history_coverage.import_id IS NULL)
   OR (core_history_coverage.from_date=EXCLUDED.from_date
       AND core_history_coverage.through_date=EXCLUDED.through_date
       AND core_history_coverage.reference=EXCLUDED.reference
       AND core_history_coverage.kind=EXCLUDED.kind
       AND core_history_coverage.origin_data_version=EXCLUDED.origin_data_version
       AND core_history_coverage.artifact_sha256=EXCLUDED.artifact_sha256
       AND core_history_coverage.import_id=EXCLUDED.import_id
       AND core_history_coverage.limitations=EXCLUDED.limitations);
