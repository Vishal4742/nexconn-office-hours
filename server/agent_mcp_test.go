package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"officehours/queue"
	"officehours/session"
	"officehours/store"
)

// /mcp as an agent's app sees it (FR-AGENT-1, FR-AGENT-4).

// mcpReply is one JSON-RPC answer from /mcp.
type mcpReply struct {
	status int
	header http.Header
	raw    string
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// mcpSend sends one 2026-07-28 request to h with the token (none when empty).
func mcpSend(t *testing.T, h http.Handler, token, method, params string) mcpReply {
	t.Helper()
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	if params != "" {
		meta += "," + params
	}
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{`+meta+`}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("MCP-Protocol-Version", "2026-07-28")
	r.Header.Set("Mcp-Method", method)
	if method == "tools/call" {
		var p struct{ Name string }
		if err := json.Unmarshal([]byte("{"+params+"}"), &p); err != nil {
			t.Fatalf("params: %v", err)
		}
		r.Header.Set("Mcp-Name", p.Name)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	body, _ := io.ReadAll(rec.Result().Body)
	out := mcpReply{status: rec.Code, header: rec.Result().Header, raw: string(body)}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("not JSON-RPC (%v): %s", err, body)
		}
	}
	return out
}

// useTool calls one tool and returns its text and whether it is an error the model should read.
func useTool(t *testing.T, h http.Handler, token, name, args string) (string, bool) {
	t.Helper()
	got := mcpSend(t, h, token, "tools/call", `"name":"`+name+`","arguments":`+args)
	if got.status != http.StatusOK || got.Result == nil {
		t.Fatalf("%s: status %d: %s", name, got.status, got.raw)
	}
	content, _ := got.Result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("%s: content %v", name, got.Result["content"])
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	return text, got.Result["isError"] == true
}

// agentWithQueue is the agent app with a live session that has one waiting hand.
func agentWithQueue(t *testing.T) *agentApp {
	t.Helper()
	a := newAgentApp(t)
	waiting := queue.Hand{ID: 31, AuthorID: otherID, Title: "Where is the cache stored?", Status: queue.Waiting}
	a.queue.hands[31] = liveSession
	a.queue.snapshot = store.Snapshot{
		Session: &store.Session{ID: liveSession, ProjectID: queueProject, Number: 3, Title: "Thursday office hours", Live: true, Version: 4, Host: person(hostID)},
		Open:    []store.QueueHand{{Hand: waiting, Author: person(otherID)}},
	}
	a.queue.applied = store.Applied{Version: 5, Before: queue.State{Live: true, Hands: []queue.Hand{waiting}}, After: queue.State{Live: true, Hands: []queue.Hand{waiting}}}
	return a
}

func TestAgentEndpointNeedsAToken(t *testing.T) {
	a := newAgentApp(t)
	good := a.connect(t, askerID)
	const challenge = `Bearer resource_metadata="https://officehours.example/.well-known/oauth-protected-resource/mcp", scope="officehours"`
	reseal := func(tok agentToken, ttl time.Duration) string {
		v, err := a.codec.Seal(accessSeal, tok, ttl)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	var tok agentToken
	if err := a.codec.Open(accessSeal, good, &tok); err != nil {
		t.Fatal(err)
	}
	code, _ := a.codec.Seal(codeSeal, authCode{GrantID: tok.GrantID, User: tok.User}, time.Minute)
	elsewhere := tok
	elsewhere.Audience = "https://other.example/mcp"

	t.Run("RFC 9728 §5.1: no token is a 401 that says where to sign in", func(t *testing.T) {
		got := mcpSend(t, a.mux, "", "tools/list", "")
		if got.status != http.StatusUnauthorized || got.header.Get("WWW-Authenticate") != challenge {
			t.Fatalf("status %d, WWW-Authenticate %q", got.status, got.header.Get("WWW-Authenticate"))
		}
	})

	t.Run("RFC 6750 §3.1: a token that can't be used is invalid_token", func(t *testing.T) {
		for name, token := range map[string]string{
			"garbage": "garbage", "expired": reseal(tok, -time.Second), "for another server": reseal(elsewhere, time.Hour),
			"a code": code, "a session cookie": a.as(t, askerID).Value,
		} {
			got := mcpSend(t, a.mux, token, "tools/list", "")
			if got.status != http.StatusUnauthorized || got.header.Get("WWW-Authenticate") != challenge+`, error="invalid_token"` {
				t.Errorf("%s: status %d, WWW-Authenticate %q", name, got.status, got.header.Get("WWW-Authenticate"))
			}
		}
	})

	t.Run("FR-AGENT-3: a disconnected app is refused on its next request", func(t *testing.T) {
		if got := mcpSend(t, a.mux, good, "tools/list", ""); got.status != http.StatusOK {
			t.Fatalf("before Disconnect: status %d: %s", got.status, got.raw)
		}
		call(a.mux, "POST", "/api/me/agents/"+tok.GrantID+"/disconnect", "{}", a.as(t, askerID))
		if got := mcpSend(t, a.mux, good, "tools/list", ""); got.status != http.StatusUnauthorized {
			t.Fatalf("after Disconnect: status %d", got.status)
		}
	})

	t.Run("another scheme than Bearer is no token", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`))
		r.SetBasicAuth("x", good)
		rec := httptest.NewRecorder()
		a.mux.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("GET is 405 without a token: there is no stream to open", func(t *testing.T) {
		rec := httptest.NewRecorder()
		a.mux.ServeHTTP(rec, httptest.NewRequest("GET", "/mcp", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status %d", rec.Code)
		}
	})
}

// agentToolNames is tools/list in its order: what agents can do (FR-AGENT-4).
var agentToolNames = []string{
	"whoami", "my_projects", "get_room", "room_members", "room_stats", "search_answers", "get_answer", "similar_answers",
	"join_room", "raise_hand", "withdraw_hand", "plus_one", "im_here",
	"register_project", "start_session", "schedule_session", "next_hand", "skip_hand", "mark_away", "merge_hand",
	"answer_hand", "remove_hand", "last_call", "end_session", "delete_answer", "mute_member",
}

func TestAgentToolList(t *testing.T) {
	a := newAgentApp(t)
	got := mcpSend(t, a.mux, a.connect(t, askerID), "tools/list", "")
	tools, _ := got.Result["tools"].([]any)
	var names []string
	destructive, readOnly := map[string]bool{}, map[string]bool{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		name := tool["name"].(string)
		names = append(names, name)
		schema := tool["inputSchema"].(map[string]any)
		if schema["type"] != "object" {
			t.Errorf("%s: inputSchema %v", name, schema)
		}
		if props, _ := schema["properties"].(map[string]any); props["hand_id"] != nil && props["hand_id"].(map[string]any)["type"] != "integer" {
			t.Errorf("%s: hand_id is not an integer", name)
		}
		if desc, _ := tool["description"].(string); len(desc) < 20 {
			t.Errorf("%s: description %q says too little for a model to choose it", name, desc)
		}
		hints := tool["annotations"].(map[string]any)
		destructive[name], readOnly[name] = hints["destructiveHint"] == true, hints["readOnlyHint"] == true
	}
	if fmt.Sprint(names) != fmt.Sprint(agentToolNames) {
		t.Errorf("tools\n%v\nwant\n%v", names, agentToolNames)
	}
	for _, name := range []string{"withdraw_hand", "remove_hand", "end_session", "delete_answer", "mute_member"} {
		if !destructive[name] {
			t.Errorf("%s is not marked destructive: the agent's app wouldn't ask its person first", name)
		}
	}
	for _, name := range []string{"whoami", "my_projects", "get_room", "room_members", "room_stats", "search_answers", "get_answer", "similar_answers"} {
		if !readOnly[name] || destructive[name] {
			t.Errorf("%s: readOnly %v destructive %v", name, readOnly[name], destructive[name])
		}
	}
}

func TestAgentActsThroughTheWebsitesOwnRules(t *testing.T) {
	t.Run("FR-AGENT-4: a contributor's agent raises a hand: the same action, card and ping as the website's", func(t *testing.T) {
		a := agentWithQueue(t)
		text, isError := useTool(t, a.mux, a.connect(t, askerID), "raise_hand",
			`{"repo":"paperplane/stash","title":"How do I pin the cache directory?","details":"Line one\nLine two"}`)
		if isError {
			t.Fatalf("raise_hand: %s", text)
		}
		last := a.queue.asked[len(a.queue.asked)-1]
		if last.Kind != queue.Raise || last.ActorID != askerID || last.Title != "How do I pin the cache directory?" || last.Details != "Line one\nLine two" {
			t.Errorf("the store was asked %+v", last)
		}
		if len(a.chat.sent) == 0 {
			t.Error("the room heard nothing: the website's raise pings the room")
		}
	})

	t.Run("NFR-SEC-2: an agent can't do what its person can't: a member's Next is the website's refusal", func(t *testing.T) {
		a := agentWithQueue(t)
		text, isError := useTool(t, a.mux, a.connect(t, askerID), "next_hand", `{"repo":"PaperPlane/Stash"}`)
		if !isError || text != "Only a maintainer of this project can do that." {
			t.Fatalf("next_hand by a member: %q (isError %v)", text, isError)
		}
		for _, act := range a.queue.asked {
			if act.Kind == queue.Next {
				t.Error("the store was asked for Next")
			}
		}
	})

	t.Run("FR-AGENT-4: a maintainer's agent takes the next hand", func(t *testing.T) {
		a := agentWithQueue(t)
		if text, isError := useTool(t, a.mux, a.connect(t, hostID), "next_hand", `{"repo":"PaperPlane/Stash"}`); isError {
			t.Fatalf("next_hand: %s", text)
		}
		if last := a.queue.asked[len(a.queue.asked)-1]; last.Kind != queue.Next || last.ActorID != hostID {
			t.Errorf("the store was asked %+v", last)
		}
	})

	t.Run("no live session: the tool says so instead of guessing one", func(t *testing.T) {
		a := agentWithQueue(t)
		a.queue.snapshot.Session.Live = false
		text, isError := useTool(t, a.mux, a.connect(t, askerID), "raise_hand", `{"repo":"PaperPlane/Stash","title":"How do I pin the cache directory?"}`)
		if !isError || !strings.Contains(text, "No session is live in PaperPlane/Stash") || len(a.queue.asked) != 0 {
			t.Fatalf("%q (isError %v), asked %v", text, isError, a.queue.asked)
		}
	})
}

func TestAgentToolInputs(t *testing.T) {
	a := agentWithQueue(t)
	token := a.connect(t, askerID)
	for name, c := range map[string]struct{ tool, args, says string }{
		"a path in a hand id":       {"withdraw_hand", `{"hand_id":"31/remove?"}`, "hand_id"},
		"a hand id that is text":    {"plus_one", `{"hand_id":"31"}`, "hand_id"},
		"a hand id of zero":         {"im_here", `{"hand_id":0}`, "hand_id"},
		"no hand id":                {"withdraw_hand", `{}`, "hand_id"},
		"a repo that isn't one":     {"get_room", `{"repo":"bad_owner/x"}`, "owner/repo"},
		"a repo with a path":        {"join_room", `{"repo":"a/b/remove"}`, "owner/repo"},
		"no repo":                   {"get_room", `{}`, "owner/repo"},
		"arguments that are a list": {"get_room", `[1]`, "arguments"},
	} {
		text, isError := useTool(t, a.mux, token, c.tool, c.args)
		if !isError || !strings.Contains(text, c.says) {
			t.Errorf("%s: %q (isError %v)", name, text, isError)
		}
	}
	if len(a.queue.asked) != 0 {
		t.Errorf("a refused input reached the store: %+v", a.queue.asked)
	}
}

// routeRecorder stands in for the website's routes: it notes each request with whoever the guard
// says made it, and answers like the real ones do in shape.
type routeRecorder struct {
	t    *testing.T
	a    *agentApp
	seen []string
}

func (rr *routeRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, err := rr.a.deps.guard.User(r)
	if err != nil {
		rr.t.Errorf("%s %s: the guard sees nobody: %v", r.Method, r.URL.Path, err)
	}
	body, _ := io.ReadAll(r.Body)
	if r.Method == "POST" && (r.Header.Get("Origin") != testSite || r.Header.Get("Content-Type") != "application/json") {
		rr.t.Errorf("%s: Origin %q, Content-Type %q: the guard would refuse it", r.URL.Path, r.Header.Get("Origin"), r.Header.Get("Content-Type"))
	}
	rr.seen = append(rr.seen, strings.TrimSpace(fmt.Sprintf("%s %s as %s %s", r.Method, r.URL.Path, u.ID, bytes.TrimSpace(body))))
	data := "{}"
	if strings.HasSuffix(r.URL.Path, "/queue") {
		data = `{"session":{"id":"12","live":true}}`
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"data":%s}`, data)
}

func TestEveryToolReachesItsOwnRoute(t *testing.T) {
	a := newAgentApp(t)
	a.faq.entries = []store.FAQEntry{faqPageFixture()}
	token := a.connect(t, hostID)
	rr := &routeRecorder{t: t, a: a}
	h := a.deps.agentEndpoint(rr, newAgentLimiter())
	const room = "/api/p/PaperPlane/Stash"
	for _, c := range []struct {
		tool, args string
		want       []string // the requests, in order; empty for a tool that reads the FAQ itself
	}{
		{"whoami", `{}`, []string{"GET /api/me as gh1"}},
		{"my_projects", `{}`, []string{"GET /api/me/projects as gh1"}},
		{"get_room", `{"repo":"PaperPlane/Stash"}`, []string{"GET " + room + " as gh1", "GET " + room + "/queue as gh1"}},
		{"room_members", `{"repo":"PaperPlane/Stash"}`, []string{"GET " + room + "/members as gh1"}},
		{"room_stats", `{"repo":"PaperPlane/Stash"}`, []string{"GET " + room + "/stats as gh1"}},
		{"search_answers", `{"repo":"PaperPlane/Stash","query":"retry"}`, nil},
		{"get_answer", `{"repo":"PaperPlane/Stash","slug":"retry-only-on-5xx"}`, nil},
		{"similar_answers", `{"repo":"PaperPlane/Stash","text":"retry on 5xx"}`, []string{"POST " + room + "/faq/similar as gh1 {\"text\":\"retry on 5xx\"}"}},
		{"join_room", `{"repo":"PaperPlane/Stash"}`, []string{"POST " + room + "/join as gh1 {}"}},
		{"raise_hand", `{"repo":"PaperPlane/Stash","title":"Why?","details":"d"}`, []string{"GET " + room + "/queue as gh1", "POST /api/sessions/12/hands as gh1 {\"title\":\"Why?\",\"details\":\"d\"}"}},
		{"withdraw_hand", `{"hand_id":31}`, []string{"POST /api/hands/31/withdraw as gh1 {}"}},
		{"plus_one", `{"hand_id":31}`, []string{"POST /api/hands/31/plus as gh1 {}"}},
		{"im_here", `{"hand_id":31}`, []string{"POST /api/hands/31/here as gh1 {}"}},
		{"register_project", `{"repo":"PaperPlane/Stash"}`, []string{"POST /api/projects as gh1 {\"owner\":\"PaperPlane\",\"repo\":\"Stash\"}"}},
		{"start_session", `{"repo":"PaperPlane/Stash","title":"Release week","minutes":45}`, []string{"POST " + room + "/sessions as gh1 {\"title\":\"Release week\",\"plannedMinutes\":45}"}},
		{"schedule_session", `{"repo":"PaperPlane/Stash","at":"2026-11-20T15:00:00Z"}`, []string{"POST " + room + "/next-session as gh1 {\"at\":\"2026-11-20T15:00:00Z\"}"}},
		{"next_hand", `{"repo":"PaperPlane/Stash"}`, []string{"GET " + room + "/queue as gh1", "POST /api/sessions/12/next as gh1 {}"}},
		{"skip_hand", `{"hand_id":31}`, []string{"POST /api/hands/31/skip as gh1 {}"}},
		{"mark_away", `{"hand_id":31}`, []string{"POST /api/hands/31/away as gh1 {}"}},
		{"merge_hand", `{"hand_id":31}`, []string{"POST /api/hands/31/merge as gh1 {}"}},
		{"answer_hand", `{"hand_id":31,"answer":"Use retryIf.","links":["https://example.com/a"]}`, []string{"POST /api/hands/31/done as gh1 {\"answer\":\"Use retryIf.\",\"links\":[\"https://example.com/a\"]}"}},
		{"remove_hand", `{"hand_id":31}`, []string{"POST /api/hands/31/remove as gh1 {}"}},
		{"last_call", `{"repo":"PaperPlane/Stash","closed":true}`, []string{"GET " + room + "/queue as gh1", "POST /api/sessions/12/lastcall as gh1 {\"closed\":true}"}},
		{"end_session", `{"repo":"PaperPlane/Stash","carry":false}`, []string{"GET " + room + "/queue as gh1", "POST /api/sessions/12/end as gh1 {\"carry\":false}"}},
		{"delete_answer", `{"repo":"PaperPlane/Stash","slug":"retry-only-on-5xx"}`, []string{"POST /api/faq/7/delete as gh1 {}"}},
		{"mute_member", `{"repo":"PaperPlane/Stash","user_id":"gh3","minutes":10}`, []string{"POST " + room + "/mute as gh1 {\"userId\":\"gh3\",\"minutes\":10}"}},
	} {
		rr.seen = nil
		text, isError := useTool(t, h, token, c.tool, c.args)
		if isError {
			t.Errorf("%s: %s", c.tool, text)
			continue
		}
		if fmt.Sprint(rr.seen) != fmt.Sprint(c.want) {
			t.Errorf("%s reached\n%q\nwant\n%q", c.tool, rr.seen, c.want)
		}
	}
	seen := map[string]bool{}
	for _, name := range agentToolNames {
		seen[name] = true
	}
	if len(seen) != 26 {
		t.Errorf("the table above must cover every tool")
	}
}

func TestAgentReadsAnswers(t *testing.T) {
	a := newAgentApp(t)
	removed := faqPageFixture()
	removed.ID, removed.Slug, removed.Deleted = 8, "old-cache-format", true
	a.faq.entries = []store.FAQEntry{faqPageFixture(), removed}
	token := a.connect(t, strangerID) // the FAQ is public: no room needed

	text, isError := useTool(t, a.mux, token, "search_answers", `{"repo":"PaperPlane/Stash","query":"retry"}`)
	if isError || !strings.Contains(text, "- [How do I retry only on 5xx errors?](https://officehours.example/p/PaperPlane/Stash/faq/retry-only-on-5xx.md): Use retryIf") {
		t.Errorf("search_answers: %q", text)
	}
	text, isError = useTool(t, a.mux, token, "get_answer", `{"repo":"PaperPlane/Stash","slug":"retry-only-on-5xx"}`)
	if isError || !strings.HasPrefix(text, "# How do I retry only on 5xx errors?\n") {
		t.Errorf("get_answer: %q", text)
	}
	if text, isError := useTool(t, a.mux, token, "search_answers", `{"repo":"nobody/nothing"}`); !isError || !strings.Contains(text, "Nobody hosts office hours for nobody/nothing") {
		t.Errorf("search_answers in an unknown project: %q", text)
	}
	if text, isError := useTool(t, a.mux, token, "search_answers", `{"repo":"PaperPlane/Stash","query":"kubernetes"}`); isError || !strings.Contains(text, `No answer of PaperPlane/Stash matches "kubernetes"`) {
		t.Errorf("search_answers with no match: %q", text)
	}
	for slug, says := range map[string]string{"old-cache-format": "removed", "no-such-answer": "no answer", "Bad_Slug/../x": "slug must be"} {
		if text, isError := useTool(t, a.mux, token, "get_answer", `{"repo":"PaperPlane/Stash","slug":"`+slug+`"}`); !isError || !strings.Contains(text, says) {
			t.Errorf("get_answer %s: %q (isError %v)", slug, text, isError)
		}
	}
}

func TestAgentMutationsAreRateLimited(t *testing.T) {
	a := agentWithQueue(t)
	token := a.connect(t, askerID)
	var refused int
	for range agentBurst + 5 {
		if text, isError := useTool(t, a.mux, token, "plus_one", `{"hand_id":31}`); isError && strings.Contains(text, "Slow down") {
			refused++
		}
	}
	if refused != 5 {
		t.Errorf("%d of %d quick +1s refused, want the 5 past the burst of %d", refused, agentBurst+5, agentBurst)
	}
	if text, isError := useTool(t, a.mux, token, "whoami", `{}`); isError {
		t.Errorf("a read was limited too: %s", text)
	}
	if text, isError := useTool(t, a.mux, a.connect(t, otherID), "plus_one", `{"hand_id":31}`); isError && strings.Contains(text, "Slow down") {
		t.Error("another person's agent was limited by this one")
	}
}

func TestAgentHearsWhatToDoWhenGitHubRefuses(t *testing.T) {
	a := agentWithQueue(t)
	a.queue.snapshot.Session.Live = false
	a.deps.starts = &sessionFakeStarts{queue: a.queue, roles: a.projects}
	a.remux()
	a.gh.status = http.StatusUnauthorized
	text, isError := useTool(t, a.mux, a.connect(t, hostID), "start_session", `{"repo":"PaperPlane/Stash","title":"Release week questions"}`)
	if !isError || !strings.Contains(text, "Sign in on the Office Hours site, then reconnect Office Hours in your app.") || strings.Contains(text, "then start the session") {
		t.Fatalf("%q (isError %v)", text, isError)
	}
}

func TestAgentEndpointLogsNoToken(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	a := agentWithQueue(t)
	token := a.connect(t, askerID)
	useTool(t, a.mux, token, "plus_one", `{"hand_id":31}`)
	mcpSend(t, a.mux, token+"x", "tools/list", "")
	if strings.Contains(buf.String(), token) || strings.Contains(buf.String(), session.Cookie) {
		t.Errorf("NFR-SEC-1: the log has the token:\n%s", buf.String())
	}
}
