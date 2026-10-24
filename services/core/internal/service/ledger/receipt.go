package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/ledger"
)

func replayCommand(ctx context.Context, commands commandStore, scope domain.Scope, actor, operation, resource, key string, body, result any) (bool, error) {
	if key == "" {
		return false, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return false, ErrUnavailable
	}
	digest := sha256.Sum256(encoded)
	payload, replay, err := commands.Reserve(ctx, scope, actor, operation, resource, key, digest[:])
	if err != nil || !replay {
		return false, err
	}
	if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) || json.Unmarshal(payload, result) != nil {
		return false, ErrUnavailable
	}
	return true, nil
}

func completeCommand(ctx context.Context, commands commandStore, scope domain.Scope, actor, operation, resource, key string, result any) error {
	if key == "" {
		return nil
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return ErrUnavailable
	}
	return commands.Complete(ctx, scope, actor, operation, resource, key, payload)
}
