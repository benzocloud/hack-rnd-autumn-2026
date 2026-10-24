package record

type Document struct {
	Version string `db:"document_version"`
	SHA256  string `db:"document_sha256"`
	Text    string `db:"document_text"`
}
