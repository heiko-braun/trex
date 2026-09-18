package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"
)

// TokenSource supplies a fresh Bearer token for an A2A call. Implemented
// by *AgentctlTokenSource for now — see specs/agent-discovery-and-registration.md's
// open question on where the token comes from once it no longer flows
// with the workflow execution as input.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// AgentctlTokenSource is a placeholder token source for local
// development and demos: it shells out to `agentctl whoami`, which
// refreshes the cached session's access token as a side effect if it
// has expired, then reads the refreshed token straight out of
// agentctl's own token cache file. This requires whoever runs the
// workflow-server to have their own `agentctl login` session on that
// machine — not viable for a real multi-user deployment, only for
// getting past the "how does the worker get a token" problem for now.
type AgentctlTokenSource struct {
	tokenFilePath string
}

// NewAgentctlTokenSource builds an AgentctlTokenSource reading from
// agentctl's default token cache path (~/.agentctl/tokens/default.yaml).
func NewAgentctlTokenSource() (*AgentctlTokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return &AgentctlTokenSource{
		tokenFilePath: filepath.Join(home, ".agentctl", "tokens", "default.yaml"),
	}, nil
}

type agentctlTokenFile struct {
	AuthToken string `yaml:"auth_token"`
}

type agentctlAuthToken struct {
	AccessToken string `json:"access_token"`
}

// Token refreshes the cached agentctl session (via `agentctl whoami`,
// which silently exchanges the refresh token if the access token has
// expired) and returns the resulting access token.
func (s *AgentctlTokenSource) Token(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "agentctl", "whoami")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("agentctl whoami: %w (output: %s)", err, out)
	}

	raw, err := os.ReadFile(s.tokenFilePath)
	if err != nil {
		return "", fmt.Errorf("read agentctl token cache %s: %w", s.tokenFilePath, err)
	}

	var file agentctlTokenFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return "", fmt.Errorf("parse agentctl token cache: %w", err)
	}

	var auth agentctlAuthToken
	if err := json.Unmarshal([]byte(file.AuthToken), &auth); err != nil {
		return "", fmt.Errorf("parse agentctl auth_token: %w", err)
	}
	if auth.AccessToken == "" {
		return "", fmt.Errorf("agentctl token cache has no access_token")
	}
	return auth.AccessToken, nil
}
