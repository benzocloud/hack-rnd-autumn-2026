package record

type SchemaVersion struct {
	Version int64 `db:"version_id"`
	Applied bool  `db:"is_applied"`
}
