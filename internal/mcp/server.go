package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func textResult(s string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}, nil, nil
}

// RunServer starts the MCP server on stdio transport.
func RunServer() error {
	mySession := resolveMySession()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "nagare",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_agents",
		Description: "List all active AI agent sessions with their status",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return textResult(ListAgentsHandler(mySession))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Description: "Send a message to another agent session. The target must be idle. This is for informational messages that don't require a reply.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SendMessageInput) (*mcp.CallToolResult, any, error) {
		return textResult(SendMessageHandler(mySession, input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message_and_wait",
		Description: "Send a message to another agent session and wait for a reply. The target must be idle. Use this when you need a response from the other agent.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SendMessageAndWaitInput) (*mcp.CallToolResult, any, error) {
		return textResult(SendMessageAndWaitHandler(ctx, mySession, input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_messages",
		Description: "Check for incoming messages from other agents and responses to your messages. Call this periodically to see if you have new messages to respond to.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return textResult(CheckMessagesHandler(mySession))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reply",
		Description: "Reply to a message you received. Use check_messages() to see your pending messages and their IDs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReplyInput) (*mcp.CallToolResult, any, error) {
		return textResult(ReplyHandler(mySession, input))
	})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}
