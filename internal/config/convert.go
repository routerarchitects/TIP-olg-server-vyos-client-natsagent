package config

import "github.com/Telecominfraproject/olg-nats-agent-core/agentcore"

func (c AppConfig) ToAgentCoreConfig() (agentcore.Config, error) {
	connectTimeout, err := parseDurationField("agentcore.nats.connect_timeout", c.AgentCore.NATS.ConnectTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	reconnectWait, err := parseDurationField("agentcore.nats.reconnect_wait", c.AgentCore.NATS.ReconnectWait, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	defaultTimeout, err := parseDurationField("agentcore.jetstream.default_timeout", c.AgentCore.JetStream.DefaultTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	kvTTL, err := parseDurationField("agentcore.kv.ttl", c.AgentCore.KV.TTL, true)
	if err != nil {
		return agentcore.Config{}, err
	}
	publishTimeout, err := parseDurationField("agentcore.timeouts.publish_timeout", c.AgentCore.Timeouts.PublishTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	subscribeTimeout, err := parseDurationField("agentcore.timeouts.subscribe_timeout", c.AgentCore.Timeouts.SubscribeTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	kvTimeout, err := parseDurationField("agentcore.timeouts.kv_timeout", c.AgentCore.Timeouts.KVTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	shutdownTimeout, err := parseDurationField("agentcore.timeouts.shutdown_timeout", c.AgentCore.Timeouts.ShutdownTimeout, false)
	if err != nil {
		return agentcore.Config{}, err
	}
	publishBackoff, err := parseDurationField("agentcore.retry.publish_backoff", c.AgentCore.Retry.PublishBackoff, false)
	if err != nil {
		return agentcore.Config{}, err
	}

	return agentcore.Config{
		AgentName: c.Agent.Name,
		Version:   c.Agent.Version,
		NATS: agentcore.NATSConfig{
			Servers:              c.AgentCore.NATS.Servers,
			ClientName:           c.AgentCore.NATS.ClientName,
			CredentialsFile:      c.AgentCore.NATS.CredentialsFile,
			NKeySeedFile:         c.AgentCore.NATS.NKeySeedFile,
			UserJWTFile:          c.AgentCore.NATS.UserJWTFile,
			Username:             c.AgentCore.NATS.Username,
			Password:             c.AgentCore.NATS.Password,
			Token:                c.AgentCore.NATS.Token,
			ConnectTimeout:       connectTimeout,
			RetryOnFailedConnect: c.AgentCore.NATS.RetryOnFailedConnect,
			MaxReconnects:        c.AgentCore.NATS.MaxReconnects,
			ReconnectWait:        reconnectWait,
			ReconnectBufSize:     c.AgentCore.NATS.ReconnectBufSize,
			TLS: &agentcore.TLSConfig{
				Enabled:            c.AgentCore.NATS.TLS.Enabled,
				InsecureSkipVerify: c.AgentCore.NATS.TLS.InsecureSkipVerify,
				CAFile:             c.AgentCore.NATS.TLS.CAFile,
				CertFile:           c.AgentCore.NATS.TLS.CertFile,
				KeyFile:            c.AgentCore.NATS.TLS.KeyFile,
				ServerName:         c.AgentCore.NATS.TLS.ServerName,
			},
		},
		JetStream: agentcore.JetStreamConfig{
			Domain:         c.AgentCore.JetStream.Domain,
			APIPrefix:      c.AgentCore.JetStream.APIPrefix,
			DefaultTimeout: defaultTimeout,
		},
		Subjects: agentcore.SubjectConfig{
			ConfigurePattern: c.AgentCore.Subjects.ConfigurePattern,
			ActionPattern:    c.AgentCore.Subjects.ActionPattern,
			ResultPattern:    c.AgentCore.Subjects.ResultPattern,
			StatusPattern:    c.AgentCore.Subjects.StatusPattern,
			HealthPattern:    c.AgentCore.Subjects.HealthPattern,
		},
		KV: agentcore.KVConfig{
			Bucket:           c.AgentCore.KV.Bucket,
			KeyPattern:       c.AgentCore.KV.KeyPattern,
			AutoCreateBucket: c.AgentCore.KV.AutoCreateBucket,
			History:          c.AgentCore.KV.History,
			TTL:              kvTTL,
			MaxValueSize:     c.AgentCore.KV.MaxValueSize,
			Storage:          c.AgentCore.KV.Storage,
			Replicas:         c.AgentCore.KV.Replicas,
		},
		Timeouts: agentcore.TimeoutConfig{
			PublishTimeout:   publishTimeout,
			SubscribeTimeout: subscribeTimeout,
			KVTimeout:        kvTimeout,
			ShutdownTimeout:  shutdownTimeout,
		},
		Retry: agentcore.RetryConfig{
			PublishAttempts: c.AgentCore.Retry.PublishAttempts,
			PublishBackoff:  publishBackoff,
		},
	}, nil
}
