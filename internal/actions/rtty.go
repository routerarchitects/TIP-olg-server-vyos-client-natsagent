package actions

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	safeTokenRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	safeHostRegex  = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)
)

// RTTYPayload represents the parameters passed to the rtty action command.
type RTTYPayload struct {
	Method   string `json:"method,omitempty"`
	Serial   string `json:"serial,omitempty"`
	ID       string `json:"id"`
	Token    string `json:"token"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	User     string `json:"user,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
	Password string `json:"password,omitempty"`
}

// Validate validates the RTTYPayload fields for security and correctness.
func (p *RTTYPayload) Validate() error {
	trimmedID := strings.TrimSpace(p.ID)
	if trimmedID == "" {
		return fmt.Errorf("%w: rtty id is required", ErrInvalidActionPayload)
	}
	if !safeTokenRegex.MatchString(trimmedID) {
		return fmt.Errorf("%w: rtty id contains invalid characters", ErrInvalidActionPayload)
	}

	trimmedToken := strings.TrimSpace(p.Token)
	if trimmedToken == "" {
		return fmt.Errorf("%w: rtty token is required", ErrInvalidActionPayload)
	}
	if !safeTokenRegex.MatchString(trimmedToken) {
		return fmt.Errorf("%w: rtty token contains invalid characters", ErrInvalidActionPayload)
	}

	trimmedServer := strings.TrimSpace(p.Server)
	if trimmedServer == "" {
		return fmt.Errorf("%w: rtty server is required", ErrInvalidActionPayload)
	}
	if !safeHostRegex.MatchString(trimmedServer) {
		return fmt.Errorf("%w: rtty server contains invalid characters", ErrInvalidActionPayload)
	}

	if p.Port <= 0 || p.Port > 65535 {
		return fmt.Errorf("%w: rtty port must be between 1 and 65535, got %d", ErrInvalidActionPayload, p.Port)
	}

	if p.Timeout < 0 {
		return fmt.Errorf("%w: rtty timeout cannot be negative", ErrInvalidActionPayload)
	}
	if p.Timeout == 0 {
		p.Timeout = 60
	}

	return nil
}
