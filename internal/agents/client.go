package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client fetches the agent catalog from the managed agents control plane.
// It holds no credential of its own: every call takes the caller's own
// token, since the control plane issues only short-lived, per-user tokens
// and the workflow server has no long-lived credential to hold instead.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against the given control-plane base URL
// (e.g. https://api.agents.sixt.cloud).
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: http.DefaultClient}
}

// agentListResponse mirrors the control plane's paged GET /agents response
// (controlplane/api/paging.go in com.sixt.service.managed-agents).
type agentListResponse struct {
	Items  []agentDTO `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// agentDTO mirrors the fields of AgentResponse
// (controlplane/api/types.go) that the workflow server needs.
type agentDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

const listPageSize = 200

// ListAgents fetches every agent visible to the tenant that token
// authenticates as, paging through the control plane's GET /agents
// endpoint.
func (c *Client) ListAgents(ctx context.Context, token string) ([]Agent, error) {
	var agents []Agent
	offset := 0
	for {
		page, total, err := c.listPage(ctx, token, offset, listPageSize)
		if err != nil {
			return nil, err
		}
		agents = append(agents, page...)
		offset += len(page)
		if len(page) == 0 || offset >= total {
			break
		}
	}
	return agents, nil
}

func (c *Client) listPage(ctx context.Context, token string, offset, limit int) ([]Agent, int, error) {
	url := fmt.Sprintf("%s/agents?limit=%d&offset=%d", c.baseURL, limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("list agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("list agents: unexpected status %d", resp.StatusCode)
	}

	var out agentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("list agents: decode response: %w", err)
	}

	agents := make([]Agent, 0, len(out.Items))
	for _, a := range out.Items {
		agents = append(agents, Agent{ID: a.ID, Name: a.Name, Type: a.Type, Enabled: a.Enabled})
	}
	return agents, out.Total, nil
}
