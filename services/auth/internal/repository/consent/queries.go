package consent

import _ "embed"

//go:embed sql/insert_evidence.sql

var insertEvidenceSQL string

//go:embed sql/active_document.sql
var activeDocumentSQL string

//go:embed sql/active_document_locked.sql
var activeDocumentLockedSQL string

//go:embed sql/ensure_document.sql
var ensureDocumentSQL string
