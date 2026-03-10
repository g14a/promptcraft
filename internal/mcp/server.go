// Package mcp implements a Model Context Protocol server for prompt enhancement.
// It provides JSON-RPC 2.0 communication over stdio for Claude MCP integration.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Enhancer transforms prompts into structured XML.
type Enhancer interface {
	Enhance(ctx context.Context, prompt, intent, targetModel string) (string, error)
}

// Config holds server configuration.
type Config struct {
	Name     string
	Version  string
	Enhancer Enhancer
}

// Server implements a Model Context Protocol server over stdio.
type Server struct {
	cfg    Config
	reader *bufio.Reader
	enc    *json.Encoder
}

// NewServer creates a new MCP stdio server.
func NewServer(cfg Config) *Server {
	return newServerIO(cfg, os.Stdin, os.Stdout)
}

// newServerIO creates a server with custom IO — used in tests.
func newServerIO(cfg Config, r io.Reader, w io.Writer) *Server {
	return &Server{
		cfg:    cfg,
		reader: bufio.NewReader(r),
		enc:    json.NewEncoder(w),
	}
}

// Run starts the server read loop. It blocks until stdin is closed or an
// unrecoverable error occurs.
func (s *Server) Run() error {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read stdin: %w", err)
		}

		if handleErr := s.handle(line); handleErr != nil {
			// Log to stderr; keep serving.
			fmt.Fprintf(os.Stderr, "handle error: %v\n", handleErr)
		}
	}
}

func (s *Server) handle(data []byte) error {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return s.writeError(nil, -32700, "parse error")
	}

	// Notifications (no ID) require no response.
	if req.ID == nil {
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	case "ping":
		return s.write(Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
	default:
		return s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req Request) error {
	return s.write(Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      ServerInfo{Name: s.cfg.Name, Version: s.cfg.Version},
		},
	})
}

func (s *Server) handleToolsList(req Request) error {
	return s.write(Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ToolsListResult{Tools: tools()},
	})
}

func (s *Server) handleToolCall(req Request) error {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.writeError(req.ID, -32602, "invalid params")
	}

	switch params.Name {
	case "enhance_prompt":
		return s.callEnhancePrompt(req.ID, params.Arguments)
	default:
		return s.writeError(req.ID, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}

func (s *Server) callEnhancePrompt(id *json.RawMessage, args map[string]any) error {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return s.writeToolError(id, "prompt is required and must be a non-empty string")
	}

	intent, _ := args["intent"].(string)
	targetModel, _ := args["target_model"].(string)

	enhanced, err := s.cfg.Enhancer.Enhance(context.Background(), prompt, intent, targetModel)
	if err != nil {
		return s.writeToolError(id, fmt.Sprintf("enhancement failed: %v", err))
	}

	return s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolCallResult{
			Content: []Content{{Type: "text", Text: enhanced}},
		},
	})
}

// write encodes r to stdout as a newline-delimited JSON object.
func (s *Server) write(r Response) error {
	return s.enc.Encode(r)
}

func (s *Server) writeError(id *json.RawMessage, code int, msg string) error {
	return s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	})
}

func (s *Server) writeToolError(id *json.RawMessage, msg string) error {
	return s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolCallResult{
			Content: []Content{{Type: "text", Text: msg}},
			IsError: true,
		},
	})
}

// tools returns the list of tools this server exposes.
func tools() []Tool {
	return []Tool{
		{
			Name: "enhance_prompt",
			Description: "Transform a raw natural language prompt into a well-structured, " +
				"XML-formatted prompt that follows Claude's best practices. Improves clarity, " +
				"adds role/context/constraints, structures instructions, and specifies output " +
				"format. Use this before sending any prompt to Claude for better results.",
			InputSchema: Schema{
				Type: "object",
				Properties: map[string]Property{
					"prompt": {
						Type:        "string",
						Description: "The raw prompt text to enhance",
					},
					"intent": {
						Type: "string",
						Description: "Optional: the underlying goal or intent behind the prompt — " +
							"helps produce a more targeted enhancement",
					},
					"target_model": {
						Type: "string",
						Description: "Optional: target Claude model tier for the enhanced prompt " +
							"(opus, sonnet, haiku). Defaults to the server's configured model.",
						Enum: []string{"opus", "sonnet", "haiku"},
					},
				},
				Required: []string{"prompt"},
			},
		},
	}
}
