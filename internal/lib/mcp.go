package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const mcpDiscoverTimeout = 15 * time.Second

type McpToolInfo struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

func DiscoverMcpTools(ctx context.Context, uri string) ([]McpToolInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, mcpDiscoverTimeout)
	defer cancel()

	c, err := client.NewStreamableHttpClient(uri)
	if err != nil {
		return nil, fmt.Errorf("create mcp client: %w", err)
	}
	defer c.Close()

	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start mcp client: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "aic3-service",
		Version: "1.0.0",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		return nil, fmt.Errorf("initialize mcp session: %w", err)
	}

	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list mcp tools: %w", err)
	}

	tools := make([]McpToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		var schema []byte
		if raw, err := json.Marshal(t.InputSchema); err == nil && string(raw) != "null" {
			schema = raw
		}
		tools = append(tools, McpToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	return tools, nil
}
