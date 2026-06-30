package config

import "gopkg.in/yaml.v3"

func MarshalRedactedYAML(c AppConfig) ([]byte, error) {
	return yaml.Marshal(c.Redacted())
}
