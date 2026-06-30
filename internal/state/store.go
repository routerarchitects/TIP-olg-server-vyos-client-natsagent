package state

import (
	"context"
	"time"
)

// State is the local persisted configure-apply checkpoint.
type State struct {
	Target      string    `json:"target"`
	AppliedUUID string    `json:"applied_uuid"`
	AppliedAt   time.Time `json:"applied_at"`
}

// Store persists and loads local configure state.
type Store interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, st State) error
}
