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

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "nagare",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_agents",
		Description: "List all active AI agent sessions with their status and unread-message count",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return textResult(ListAgentsHandler(resolveMySession()))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Description: "Send a message to another agent session. The target must be idle. This is for informational messages that don't require a reply.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SendMessageInput) (*mcp.CallToolResult, any, error) {
		return textResult(SendMessageHandler(resolveMySession(), input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message_and_wait",
		Description: "Persist a message for an idle agent, notify its pane, and wait for a reply. Returns after 30 seconds if the target does not acknowledge reading it; after acknowledgment, waits up to timeout for the reply.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SendMessageAndWaitInput) (*mcp.CallToolResult, any, error) {
		return textResult(SendMessageAndWaitHandler(ctx, resolveMySession(), input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_messages",
		Description: "Check incoming messages, outgoing delivery/read state, and replies. Incoming reads durably acknowledge delivery.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		return textResult(CheckMessagesHandler(resolveMySession()))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reply",
		Description: "Reply to a message you received. Use check_messages() to see your pending messages and their IDs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReplyInput) (*mcp.CallToolResult, any, error) {
		return textResult(ReplyHandler(resolveMySession(), input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tickets",
		Description: "List Nagare tickets, optionally filtered by status, project, or today's work",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListTicketsInput) (*mcp.CallToolResult, any, error) {
		return textResult(ListTicketsHandler(input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_ticket",
		Description: "Get the full context for a Nagare ticket by ID or unique ID prefix",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetTicketInput) (*mcp.CallToolResult, any, error) {
		return textResult(GetTicketHandler(input))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "submit_ticket",
		Description: "Submit an assigned running ticket for human review. The summary must describe the completed work and verification; Nagare records the agent, session, repository, and submission time.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubmitTicketInput) (*mcp.CallToolResult, any, error) {
		return textResult(SubmitTicketHandler(resolveMySession(), input))
	})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}
