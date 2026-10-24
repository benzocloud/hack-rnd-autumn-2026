package consent

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/service/account"
)

//go:embed personal-data-consent.md
var text string

func Document() account.Document {
	sum := sha256.Sum256([]byte(text))
	return account.Document{Text: text, Version: "mock-2026-10-24-v1", SHA256: hex.EncodeToString(sum[:])}
}
