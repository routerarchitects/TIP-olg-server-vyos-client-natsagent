#!/usr/bin/env bash
set -euo pipefail

# Config validation smoke check
#
# What this validates:
# 1. Config path resolution and YAML load
# 2. Defaults + explicit YAML override behavior
# 3. Config validation rules
# 4. Conversion to agentcore.Config
#
# Command executed:
# go run ./cmd/vyos-nats-agent \
#   --config ./config.example.yaml \
#   --validate-config
#
# Expected success indicator:
# configuration valid
#
# To inspect sanitized effective config manually:
# go run ./cmd/vyos-nats-agent --config ./config.example.yaml --print-effective-config --validate-config

go run ./cmd/vyos-nats-agent \
  --config ./config.example.yaml \
  --validate-config
