// Package mcp answers the Model Context Protocol over Streamable HTTP on one endpoint. It speaks
// the 2026-07-28 revision, where every request carries its version in _meta, and the legacy ones
// (2025-03-26 to 2025-11-25), which open with initialize. It knows the protocol and nothing of
// Office Hours: the tools and what they do come from the caller, and so does sign-in, which is
// checked before a request gets here. Every reply is one JSON object: no SSE stream, no session,
// nothing sent unasked.
package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

// The versions this server speaks, the newest first: the modern revision, then the legacy ones.
var (
	modernVersions = []string{"2026-07-28"}
	legacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}
	allVersions    = slices.Concat(modernVersions, legacyVersions)
)

const (
	// maxBody is the largest request taken. A tool call carries a question or an answer of a few
	// thousand characters at most.
	maxBody = 64 << 10
	// listTTL is how long a client may keep the tool list and the discovery result.
	listTTL = time.Hour
)

// JSON-RPC's error codes, and the ones MCP reserves for itself.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
	codeHeaderMismatch = -32020
	codeUnsupported    = -32022
)

// capabilities says what the server offers: tools, and nothing it would send unasked.
var capabilities = map[string]any{"tools": map[string]any{}}

// Info names the server, in discovery, in initialize and in every result's _meta.
type Info struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Annotations are a tool's hints to the agent's app, which may ask the person before a
// destructive one. All four are always written: when one is missing, a client must assume the
// worst (destructive, open world).
type Annotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

// Tool is one tool as tools/list describes it.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations Annotations    `json:"annotations"`
}

// Result is what a tool answers: text for the model. IsError marks a refusal or a failure the
// model should read and act on; it is still a result, not a protocol error.
type Result struct {
	Text    string
	IsError bool
}

// Server answers MCP requests. Call runs one of Tools with the request's context, which carries
// whoever signed in.
type Server struct {
	Info         Info
	Instructions string
	Origin       string // the one Origin a browser may send (DNS rebinding); a request without one passes
	Tools        []Tool
	Call         func(ctx context.Context, name string, args json.RawMessage) Result
}

// message is one JSON-RPC request or notification.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"` // absent: a notification
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// params holds what any method here reads from its params.
type params struct {
	Meta            map[string]json.RawMessage `json:"_meta"`
	Name            string                     `json:"name"`            // tools/call, prompts/get
	URI             string                     `json:"uri"`             // resources/read
	Arguments       json.RawMessage            `json:"arguments"`       // tools/call
	ProtocolVersion string                     `json:"protocolVersion"` // initialize
}

// rpcError is a JSON-RPC error and the HTTP status it goes out on.
type rpcError struct {
	status  int
	code    int
	message string
	data    any
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if o := r.Header.Get("Origin"); o != "" && o != s.Origin {
		s.reply(w, nil, nil, &rpcError{http.StatusForbidden, codeInvalidRequest, "This server takes no requests from other websites.", nil})
		return
	}
	m, fail := decode(w, r)
	if fail != nil {
		s.reply(w, nil, nil, fail)
		return
	}
	if len(m.ID) == 0 { // a notification: nothing here needs one, and nothing answers it
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, fail := s.answer(r, m)
	s.reply(w, m.ID, result, fail)
}

// decode reads one JSON-RPC message from the body.
func decode(w http.ResponseWriter, r *http.Request) (message, *rpcError) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		return message{}, &rpcError{http.StatusBadRequest, codeParse, "The request is too large or could not be read.", nil}
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		return message{}, &rpcError{http.StatusBadRequest, codeInvalidRequest, "Send one request per POST: batches are not supported.", nil}
	}
	var m message
	if err := json.Unmarshal(raw, &m); err != nil {
		return message{}, &rpcError{http.StatusBadRequest, codeParse, "The request is not valid JSON.", nil}
	}
	if m.JSONRPC != "2.0" || m.Method == "" {
		return message{}, &rpcError{http.StatusBadRequest, codeInvalidRequest, `A request needs "jsonrpc": "2.0" and a method.`, nil}
	}
	if len(m.ID) > 0 && !validID(m.ID) {
		return message{}, &rpcError{http.StatusBadRequest, codeInvalidRequest, "A request id is a string or an integer.", nil}
	}
	return m, nil
}

// validID: MCP takes a string or an integer, and never null.
func validID(id json.RawMessage) bool {
	if string(id) == "null" {
		return false
	}
	var s string
	var n int64
	return json.Unmarshal(id, &s) == nil || json.Unmarshal(id, &n) == nil
}

// answer runs a request and returns its result.
func (s *Server) answer(r *http.Request, m message) (map[string]any, *rpcError) {
	var p params
	if len(m.Params) > 0 {
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, &rpcError{http.StatusBadRequest, codeInvalidParams, "params must be an object.", nil}
		}
	}
	if m.Method == "initialize" { // legacy: opens a session this server doesn't need to keep
		return s.initialize(p), nil
	}
	isModern, fail := checkVersion(r, m, p)
	if fail != nil {
		return nil, fail
	}
	switch m.Method {
	case "server/discover":
		return map[string]any{
			"supportedVersions": allVersions, "capabilities": capabilities, "instructions": s.Instructions,
			"ttlMs": listTTL.Milliseconds(), "cacheScope": "public",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": slices.Clip(append([]Tool{}, s.Tools...)), "ttlMs": listTTL.Milliseconds(), "cacheScope": "public"}, nil
	case "tools/call":
		return s.call(r.Context(), p)
	}
	status := http.StatusOK // a legacy client reads a JSON-RPC error only on a 2xx
	if isModern {
		status = http.StatusNotFound
	}
	return nil, &rpcError{status, codeNoMethod, "Method not found: " + m.Method, nil}
}

// initialize answers a legacy client's handshake with the version it asked for when that is a
// legacy one we speak, else the newest legacy one.
func (s *Server) initialize(p params) map[string]any {
	version := legacyVersions[0]
	if slices.Contains(legacyVersions, p.ProtocolVersion) {
		version = p.ProtocolVersion
	}
	return map[string]any{"protocolVersion": version, "capabilities": capabilities, "serverInfo": s.Info, "instructions": s.Instructions}
}

// checkVersion decides the request's era and checks what that era asks for. A modern request
// carries its version in _meta, and its version, method and tool name again in headers, which
// must match the body, since a gateway may route on them. A legacy one names a legacy version in
// its header, or none for 2025-03-26.
func checkVersion(r *http.Request, m message, p params) (isModern bool, _ *rpcError) {
	header := r.Header.Get("MCP-Protocol-Version")
	if method := r.Header.Get("Mcp-Method"); method != "" && method != m.Method {
		return false, mismatch("Mcp-Method is %q but the body's method is %q", method, m.Method)
	}
	raw, isModern := p.Meta["io.modelcontextprotocol/protocolVersion"]
	if !isModern {
		switch {
		case header == "" || slices.Contains(legacyVersions, header):
			return false, nil
		case slices.Contains(modernVersions, header):
			return false, mismatch("MCP-Protocol-Version is %s but the body has no _meta protocol version", header)
		}
		return false, unsupported(header)
	}
	var version string
	if json.Unmarshal(raw, &version) != nil || version == "" {
		return true, &rpcError{http.StatusBadRequest, codeInvalidParams, "The _meta protocol version must be a string.", nil}
	}
	if header != version {
		return true, mismatch("MCP-Protocol-Version is %q but the _meta protocol version is %q", header, version)
	}
	if r.Header.Get("Mcp-Method") == "" {
		return true, mismatch("the Mcp-Method header is missing")
	}
	if want, named := nameOf(m.Method, p); named {
		if got, err := headerValue(r.Header.Get("Mcp-Name")); err != nil || got != want {
			return true, mismatch("Mcp-Name does not match the body's %q", want)
		}
	}
	if caps := bytes.TrimSpace(p.Meta["io.modelcontextprotocol/clientCapabilities"]); len(caps) == 0 || caps[0] != '{' {
		return true, &rpcError{http.StatusBadRequest, codeInvalidParams, "_meta needs io.modelcontextprotocol/clientCapabilities.", nil}
	}
	if !slices.Contains(allVersions, version) {
		return true, unsupported(version)
	}
	return true, nil
}

// nameOf is what the Mcp-Name header must repeat for methods that name something.
func nameOf(method string, p params) (string, bool) {
	switch method {
	case "tools/call", "prompts/get":
		return p.Name, true
	case "resources/read":
		return p.URI, true
	}
	return "", false
}

// headerValue undoes the "=?base64?…?=" form a client uses for a value a header can't carry.
func headerValue(v string) (string, error) {
	encoded, ok := strings.CutPrefix(v, "=?base64?")
	if !ok || !strings.HasSuffix(encoded, "?=") {
		return v, nil
	}
	plain, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(encoded, "?="))
	return string(plain), err
}

func mismatch(format string, args ...any) *rpcError {
	return &rpcError{http.StatusBadRequest, codeHeaderMismatch, "Header mismatch: " + fmt.Sprintf(format, args...), nil}
}

func unsupported(version string) *rpcError {
	return &rpcError{http.StatusBadRequest, codeUnsupported, "Unsupported protocol version",
		map[string]any{"supported": allVersions, "requested": version}}
}

// call runs a tool. An unknown name is a protocol error; whatever the tool says, refusals too,
// is a result.
func (s *Server) call(ctx context.Context, p params) (map[string]any, *rpcError) {
	if !slices.ContainsFunc(s.Tools, func(t Tool) bool { return t.Name == p.Name }) {
		return nil, &rpcError{http.StatusOK, codeInvalidParams, "Unknown tool: " + p.Name, nil}
	}
	args := p.Arguments
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	res := s.Call(ctx, p.Name, args)
	return map[string]any{"content": []map[string]any{{"type": "text", "text": res.Text}}, "isError": res.IsError}, nil
}

// reply writes a result, which names its type and the server, or an error. An id that could not
// be read is left out.
func (s *Server) reply(w http.ResponseWriter, id json.RawMessage, result map[string]any, fail *rpcError) {
	out := map[string]any{"jsonrpc": "2.0"}
	if len(id) > 0 {
		out["id"] = id
	}
	status := http.StatusOK
	if fail != nil {
		status = fail.status
		e := map[string]any{"code": fail.code, "message": fail.message}
		if fail.data != nil {
			e["data"] = fail.data
		}
		out["error"] = e
	} else {
		result["resultType"] = "complete"
		result["_meta"] = map[string]any{"io.modelcontextprotocol/serverInfo": s.Info}
		out["result"] = result
	}
	body, err := json.Marshal(out)
	if err != nil {
		log.Printf("mcp: reply: %v", err)
		http.Error(w, "Something went wrong on our side.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Printf("mcp: write: %v", err)
	}
}
