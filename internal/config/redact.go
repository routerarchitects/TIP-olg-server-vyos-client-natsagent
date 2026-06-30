package config

import (
	"net/url"
	"strings"
)

const redactedValue = "********"

func (c AppConfig) Redacted() AppConfig {
	out := c

	out.AgentCore.NATS.Username = redactString(out.AgentCore.NATS.Username)
	out.AgentCore.NATS.Password = redactString(out.AgentCore.NATS.Password)
	out.AgentCore.NATS.Token = redactString(out.AgentCore.NATS.Token)
	out.AgentCore.NATS.CredentialsFile = redactString(out.AgentCore.NATS.CredentialsFile)
	out.AgentCore.NATS.NKeySeedFile = redactString(out.AgentCore.NATS.NKeySeedFile)
	out.AgentCore.NATS.UserJWTFile = redactString(out.AgentCore.NATS.UserJWTFile)
	out.AgentCore.NATS.TLS.KeyFile = redactString(out.AgentCore.NATS.TLS.KeyFile)
	out.AgentCore.NATS.TLS.CertFile = redactString(out.AgentCore.NATS.TLS.CertFile)
	out.AgentCore.NATS.TLS.CAFile = redactString(out.AgentCore.NATS.TLS.CAFile)

	if len(out.AgentCore.NATS.Servers) > 0 {
		redactedServers := make([]string, len(out.AgentCore.NATS.Servers))
		for i, s := range out.AgentCore.NATS.Servers {
			u, err := url.Parse(s)
			if err == nil && u != nil && u.User != nil {
				u.User = url.UserPassword("redacted", "redacted")
				redactedServers[i] = u.String()
			} else {
				redactedServers[i] = s
			}
		}
		out.AgentCore.NATS.Servers = redactedServers
	}

	return out
}

func redactString(v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return redactedValue
}
