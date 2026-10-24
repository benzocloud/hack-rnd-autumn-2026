INSERT INTO consent_documents(singleton,document_version,document_sha256,document_text) VALUES(true,$1,$2,$3) ON CONFLICT(singleton) DO NOTHING;
