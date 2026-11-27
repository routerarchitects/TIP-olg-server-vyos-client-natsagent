package state

import (
	"context"
	"time"
)

// State is the local persisted configure-apply checkpoint.
type State struct {
	Target            string            `json:"target"`
	AppliedUUID       string            `json:"applied_uuid"`
	AppliedAt         time.Time         `json:"applied_at"`
	PhysicalPortRoles map[string]string `json:"physical_port_roles,omitempty"`
	TopologyDegraded  bool              `json:"topology_degraded,omitempty"`
}

// IsZero reports whether s is the zero State value.
func (s State) IsZero() bool {
	return s.Target == "" && s.AppliedUUID == "" && s.AppliedAt.IsZero() && len(s.PhysicalPortRoles) == 0 && !s.TopologyDegraded
}

// Store persists and loads local configure state.
type Store interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, st State) error
}
