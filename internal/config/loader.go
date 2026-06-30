package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func ResolvePath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if envPath := os.Getenv(envConfigPathKey); envPath != "" {
		return envPath
	}
	return defaultConfigPath
}

func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	cfg := DefaultAppConfig()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func LoadResolved(flagPath string) (*AppConfig, string, error) {
	path := ResolvePath(flagPath)
	cfg, err := Load(path)
	if err != nil {
		return nil, path, err
	}
	return cfg, path, nil
}
