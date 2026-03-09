package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- Test double -------------------------------------------------------------

// mockEnhancer is a controllable implementation of the Enhancer interface.
type mockEnhancer struct {
	result string
	err    error
}

func (m *mockEnhancer) Enhance(_ context.Context, prompt, _, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.result != "" {
		return m.result, nil
	}
	return "<instructions>" + prompt + "</instructions>", nil
}

// --- Test helpers ------------------------------------------------------------

// newTestServer builds a server with injectable output buffer.
func newTestServer(enh Enhancer) (*Server, *bytes.Buffer) {
	out := &bytes.Buffer{}
	srv := newServerIO(
		Config{Name: "test-server", Version: "0.0.1", Enhancer: enh},
		strings.NewReader(""), // stdin unused in handle() tests
		out,
	)
	return srv, out
}

// handle sends one raw JSON line through the server and returns the output buffer.
func send(t *testing.T, srv *Server, msg any) *bytes.Buffer {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	out := &bytes.Buffer{}
	srv.enc = json.NewEncoder(out)
	if err := srv.handle(data); err != nil {
		t.Fatalf("handle error: %v", err)
	}
	return out
}

// decodeResp decodes the single JSON response in buf.
func decodeResp(t *testing.T, buf *bytes.Buffer) Response {
	t.Helper()
	var r Response
	if err := json.NewDecoder(buf).Decode(&r); err != nil {
		t.Fatalf("decode response: %v (raw: %s)", err, buf.String())
	}
	return r
}

// rawID returns a JSON-encoded integer ID.
func rawID(n int) *json.RawMessage {
	b := json.RawMessage(fmt.Sprintf("%d", n))
	return &b
}

// ---- initialize -------------------------------------------------------------

func TestHandleInitialize(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(1),
		Method:  "initialize",
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}

	// Decode result as InitializeResult.
	rb, _ := json.Marshal(resp.Result)
	var result InitializeResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode InitializeResult: %v", err)
	}

	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion = %q; want %q", result.ProtocolVersion, "2024-11-05")
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q; want %q", result.ServerInfo.Name, "test-server")
	}
	if result.ServerInfo.Version != "0.0.1" {
		t.Errorf("ServerInfo.Version = %q; want %q", result.ServerInfo.Version, "0.0.1")
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Error("capabilities missing 'tools' key")
	}
}

// ---- tools/list -------------------------------------------------------------

func TestHandleToolsList(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(2),
		Method:  "tools/list",
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	rb, _ := json.Marshal(resp.Result)
	var result ToolsListResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode ToolsListResult: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Fatal("tools list is empty")
	}

	// Must expose enhance_prompt.
	var found bool
	for _, tool := range result.Tools {
		if tool.Name == "enhance_prompt" {
			found = true
			if tool.Description == "" {
				t.Error("enhance_prompt.Description is empty")
			}
			if len(tool.InputSchema.Required) == 0 {
				t.Error("enhance_prompt.InputSchema.Required is empty")
			}
			if _, ok := tool.InputSchema.Properties["prompt"]; !ok {
				t.Error("enhance_prompt schema missing 'prompt' property")
			}
		}
	}
	if !found {
		t.Error("tools/list did not include 'enhance_prompt'")
	}
}

// ---- tools/call: enhance_prompt ---------------------------------------------

func TestHandleToolCallEnhancePrompt(t *testing.T) {
	const enhanced = "<instructions>fixed!</instructions>"
	srv, _ := newTestServer(&mockEnhancer{result: enhanced})

	params, _ := json.Marshal(ToolCallParams{
		Name:      "enhance_prompt",
		Arguments: map[string]any{"prompt": "fix the bug"},
	})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(3),
		Method:  "tools/call",
		Params:  params,
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %+v", resp.Error)
	}

	rb, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode ToolCallResult: %v", err)
	}

	if result.IsError {
		t.Errorf("IsError = true; want false")
	}
	if len(result.Content) == 0 {
		t.Fatal("Content is empty")
	}
	if result.Content[0].Text != enhanced {
		t.Errorf("Content[0].Text = %q; want %q", result.Content[0].Text, enhanced)
	}
}

func TestHandleToolCallEnhancePromptEmptyPrompt(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	params, _ := json.Marshal(ToolCallParams{
		Name:      "enhance_prompt",
		Arguments: map[string]any{"prompt": ""},
	})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(4),
		Method:  "tools/call",
		Params:  params,
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error (want tool error, not RPC error): %+v", resp.Error)
	}

	rb, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode ToolCallResult: %v", err)
	}

	if !result.IsError {
		t.Error("IsError = false; want true for empty prompt")
	}
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		t.Error("expected non-empty error message in Content[0].Text")
	}
}

func TestHandleToolCallEnhancerError(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{err: fmt.Errorf("nlp failure")})

	params, _ := json.Marshal(ToolCallParams{
		Name:      "enhance_prompt",
		Arguments: map[string]any{"prompt": "fix this"},
	})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(5),
		Method:  "tools/call",
		Params:  params,
	})

	resp := decodeResp(t, buf)

	rb, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode ToolCallResult: %v", err)
	}

	if !result.IsError {
		t.Error("IsError = false; want true when enhancer fails")
	}
}

func TestHandleToolCallWithIntentAndModel(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	params, _ := json.Marshal(ToolCallParams{
		Name: "enhance_prompt",
		Arguments: map[string]any{
			"prompt":       "explain goroutines",
			"intent":       "for a beginner",
			"target_model": "sonnet",
		},
	})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(6),
		Method:  "tools/call",
		Params:  params,
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	rb, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	if err := json.Unmarshal(rb, &result); err != nil {
		t.Fatalf("decode ToolCallResult: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true; want false (intent/model are optional, not errors)")
	}
}

// ---- unknown tool -----------------------------------------------------------

func TestHandleToolCallUnknownTool(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	params, _ := json.Marshal(ToolCallParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]any{},
	})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(7),
		Method:  "tools/call",
		Params:  params,
	})

	resp := decodeResp(t, buf)
	// Unknown tool returns a JSON-RPC error, not a tool error.
	if resp.Error == nil {
		t.Error("expected JSON-RPC error for unknown tool; got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d; want -32601", resp.Error.Code)
	}
}

// ---- ping -------------------------------------------------------------------

func TestHandlePing(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(8),
		Method:  "ping",
	})

	resp := decodeResp(t, buf)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

// ---- unknown method ---------------------------------------------------------

func TestHandleUnknownMethod(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})

	buf := send(t, srv, Request{
		JSONRPC: "2.0",
		ID:      rawID(9),
		Method:  "rpc.unknownMethod",
	})

	resp := decodeResp(t, buf)
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error; got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d; want -32601 (method not found)", resp.Error.Code)
	}
}

// ---- notifications (no ID) --------------------------------------------------

func TestHandleNotificationProducesNoOutput(t *testing.T) {
	srv, out := newTestServer(&mockEnhancer{})

	// Notifications have no ID field.
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	data, _ := json.Marshal(notification)

	if err := srv.handle(data); err != nil {
		t.Fatalf("handle notification error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("notification produced output %q; want empty", out.String())
	}
}

// ---- parse error ------------------------------------------------------------

func TestHandleInvalidJSON(t *testing.T) {
	srv, _ := newTestServer(&mockEnhancer{})
	out := &bytes.Buffer{}
	srv.enc = json.NewEncoder(out)

	if err := srv.handle([]byte(`{not valid json}`)); err != nil {
		t.Fatalf("handle should not return error (it writes the error to output): %v", err)
	}

	resp := decodeResp(t, out)
	if resp.Error == nil {
		t.Fatal("expected parse error response; got nil error")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d; want -32700 (parse error)", resp.Error.Code)
	}
}

// ---- Run: EOF terminates cleanly --------------------------------------------

func TestRunReturnsNilOnEOF(t *testing.T) {
	out := &bytes.Buffer{}
	srv := newServerIO(
		Config{Name: "t", Version: "0", Enhancer: &mockEnhancer{}},
		strings.NewReader(""), // empty reader → immediate EOF
		out,
	)
	if err := srv.Run(); err != nil {
		t.Errorf("Run() = %v; want nil on EOF", err)
	}
}

// ---- Benchmarks -------------------------------------------------------------

func BenchmarkHandleInitialize(b *testing.B) {
	srv, _ := newTestServer(&mockEnhancer{})
	data, _ := json.Marshal(Request{JSONRPC: "2.0", ID: rawID(1), Method: "initialize"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &bytes.Buffer{}
		srv.enc = json.NewEncoder(out)
		_ = srv.handle(data)
	}
}

func BenchmarkHandleToolCall(b *testing.B) {
	const enhanced = "<role>Expert</role>\n<instructions>Fix the bug.</instructions>"
	srv, _ := newTestServer(&mockEnhancer{result: enhanced})

	params, _ := json.Marshal(ToolCallParams{
		Name:      "enhance_prompt",
		Arguments: map[string]any{"prompt": "fix the authentication bug"},
	})
	data, _ := json.Marshal(Request{
		JSONRPC: "2.0",
		ID:      rawID(1),
		Method:  "tools/call",
		Params:  params,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := &bytes.Buffer{}
		srv.enc = json.NewEncoder(out)
		_ = srv.handle(data)
	}
}

func BenchmarkHandleToolCallParallel(b *testing.B) {
	const enhanced = "<instructions>Done.</instructions>"
	params, _ := json.Marshal(ToolCallParams{
		Name:      "enhance_prompt",
		Arguments: map[string]any{"prompt": "fix the bug"},
	})
	data, _ := json.Marshal(Request{
		JSONRPC: "2.0",
		ID:      rawID(1),
		Method:  "tools/call",
		Params:  params,
	})

	b.RunParallel(func(pb *testing.PB) {
		srv, _ := newTestServer(&mockEnhancer{result: enhanced})
		for pb.Next() {
			out := &bytes.Buffer{}
			srv.enc = json.NewEncoder(out)
			_ = srv.handle(data)
		}
	})
}
