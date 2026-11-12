package nexconn

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testKey    = "appkey_test"
	testSecret = "secret_test"
)

// call is what the fake Nexconn API saw.
type call struct {
	path   string
	header http.Header
	body   map[string]any
}

// fakeAPI answers every request with status and reply, and records the last call.
func fakeAPI(t *testing.T, status int, reply string) (*Client, *call) {
	t.Helper()
	got := &call{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.path, got.header, got.body = r.URL.Path, r.Header.Clone(), map[string]any{}
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Errorf("request body is not JSON: %q", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Config{AppKey: testKey, AppSecret: testSecret, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, got
}

func TestNewNeedsKeySecretAndBaseURL(t *testing.T) {
	for _, cfg := range []Config{
		{AppSecret: testSecret, BaseURL: "https://api.example"},
		{AppKey: testKey, BaseURL: "https://api.example"},
		{AppKey: testKey, AppSecret: testSecret},
	} {
		_, err := New(cfg)
		if err == nil {
			t.Fatalf("New(%+v) = nil error, want one", cfg)
		}
		if strings.Contains(err.Error(), testSecret) {
			t.Fatalf("NFR-SEC-1: the error text contains the secret: %v", err)
		}
	}
}

func TestIssueTokenSignsTheRequest(t *testing.T) {
	c, got := fakeAPI(t, 200, `{"code":0,"result":{"userId":"gh42","accessToken":"tok_abc"}}`)
	token, err := c.IssueToken(context.Background(), "gh42", "Mia Kowalski")
	if err != nil || token != "tok_abc" {
		t.Fatalf("IssueToken = %q, %v; want tok_abc, nil", token, err)
	}
	if got.path != "/v4/auth/access-token/issue" {
		t.Errorf("path = %q", got.path)
	}
	if got.body["userId"] != "gh42" || got.body["name"] != "Mia Kowalski" {
		t.Errorf("body = %v", got.body)
	}
	nonce, ts := got.header.Get("Nonce"), got.header.Get("Timestamp")
	sum := sha1.Sum([]byte(testSecret + nonce + ts))
	if got.header.Get("App-Key") != testKey || nonce == "" || ts == "" || got.header.Get("Signature") != hex.EncodeToString(sum[:]) {
		t.Errorf("FR-AUTH-2: request is not signed as sha1(secret+nonce+timestamp): %v", got.header)
	}
}

func TestCreateAndJoinGroup(t *testing.T) {
	c, got := fakeAPI(t, 200, `{"code":0}`)
	if err := c.CreateGroup(context.Background(), "p17", "paperplane/stash", "oh_bot"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if got.path != "/v4/group-channel/create" || got.body["channelId"] != "p17" || got.body["name"] != "paperplane/stash" || got.body["owner"] != "oh_bot" {
		t.Errorf("FR-PROJ-2: create sent %s %v", got.path, got.body)
	}

	c, got = fakeAPI(t, 200, `{"code":0,"result":{"channelId":"p17","memberCount":3}}`)
	if err := c.JoinGroup(context.Background(), "p17", "gh1", "gh2"); err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}
	ids, _ := got.body["userIds"].([]any)
	if got.path != "/v4/group-channel/join" || got.body["channelId"] != "p17" || len(ids) != 2 || ids[0] != "gh1" || ids[1] != "gh2" {
		t.Errorf("FR-PROJ-4: join sent %s %v", got.path, got.body)
	}
}

func TestSend(t *testing.T) {
	const reply = `{"code":0,"result":{"messageIds":[{"channelId":"p17","messageId":"MSG-1"}]}}`
	tests := []struct {
		name string
		msg  Message
		want map[string]any // fields the request must carry
		not  []string       // fields the request must not carry
	}{
		{
			name: "FR-HAND-5: a stored card with push text",
			msg:  Message{From: "oh_bot", ChannelID: "p17", Type: "OH:Hand", Content: `{"handId":"h1"}`, Push: "Ana raised a hand"},
			want: map[string]any{"fromUserId": "oh_bot", "messageType": "OH:Hand", "content": `{"handId":"h1"}`, "pushContent": "Ana raised a hand", "shouldPersist": float64(1)},
			not:  []string{"hasMention", "disablePush", "disableUpdateLastMsg"},
		},
		{
			name: "FR-QUEUE-4: the queue ping is not stored, not pushed and leaves the last message alone",
			msg:  Message{From: "oh_bot", ChannelID: "p17", Type: "OH:Queue", Content: `{"sessionId":"s1","version":7}`, Transient: true},
			want: map[string]any{"messageType": "OH:Queue", "shouldPersist": float64(0), "disablePush": true, "disableUpdateLastMsg": true},
			not:  []string{"pushContent", "hasMention"},
		},
		{
			name: "FR-SESS-3: a mention sets hasMention",
			msg:  Message{From: "oh_bot", ChannelID: "p17", Type: "RC:TxtMsg", Content: `{"content":"@all live","mentionedInfo":{"type":1}}`, Mention: true},
			want: map[string]any{"messageType": "RC:TxtMsg", "hasMention": float64(1), "shouldPersist": float64(1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, got := fakeAPI(t, 200, reply)
			id, err := c.Send(context.Background(), tt.msg)
			if err != nil || id != "MSG-1" {
				t.Fatalf("Send = %q, %v; want MSG-1, nil", id, err)
			}
			to, _ := got.body["toChannelIds"].([]any)
			if got.path != "/v4/group-channel/message/send" || len(to) != 1 || to[0] != "p17" {
				t.Errorf("sent %s to %v", got.path, got.body["toChannelIds"])
			}
			for k, v := range tt.want {
				if got.body[k] != v {
					t.Errorf("%s = %v (%T), want %v", k, got.body[k], got.body[k], v)
				}
			}
			for _, k := range tt.not {
				if _, ok := got.body[k]; ok {
					t.Errorf("%s should not be sent, got %v", k, got.body[k])
				}
			}
		})
	}
}

func TestMute(t *testing.T) {
	c, got := fakeAPI(t, 200, `{"code":0}`)
	if err := c.Mute(context.Background(), "p17", "gh9", 60); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	ids, _ := got.body["userIds"].([]any)
	if got.path != "/v4/group-channel/user/mute-list/add" || got.body["channelId"] != "p17" || len(ids) != 1 || ids[0] != "gh9" || got.body["durationMinutes"] != float64(60) {
		t.Errorf("FR-MOD-1: mute sent %s %v", got.path, got.body)
	}
}

func TestErrors(t *testing.T) {
	t.Run("an HTTP error carries status, code and message", func(t *testing.T) {
		c, _ := fakeAPI(t, 401, `{"code":1004,"errorMessage":"signature error"}`)
		_, err := c.IssueToken(context.Background(), "gh1", "A")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Code != 1004 || apiErr.Msg != "signature error" || apiErr.Op != "issue token" {
			t.Fatalf("err = %#v, want *Error{issue token, 401, 1004, signature error}", err)
		}
		if strings.Contains(err.Error(), testSecret) {
			t.Fatalf("NFR-SEC-1: the error text contains the secret: %v", err)
		}
	})
	t.Run("a non-zero code in a 200 reply is an error", func(t *testing.T) {
		c, _ := fakeAPI(t, 200, `{"code":25001}`)
		err := c.CreateGroup(context.Background(), "p17", "n", "oh_bot")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != 25001 || apiErr.Status != 200 {
			t.Fatalf("err = %#v, want *Error with code 25001", err)
		}
	})
	t.Run("FR-PROJ-2: creating a group that already exists is not an error", func(t *testing.T) {
		c, _ := fakeAPI(t, 400, `{"code":25409,"errorMessage":"Group channel ID already exists"}`)
		if err := c.CreateGroup(context.Background(), "p17", "n", "oh_bot"); err != nil {
			t.Fatalf("CreateGroup on an existing id = %v, want nil", err)
		}
	})
	t.Run("a token reply without a token is an error", func(t *testing.T) {
		c, _ := fakeAPI(t, 200, `{"code":0,"result":{"userId":"gh1"}}`)
		if _, err := c.IssueToken(context.Background(), "gh1", "A"); err == nil {
			t.Fatal("IssueToken = nil error, want one")
		}
	})
	t.Run("a slow API is cut off by the timeout", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		t.Cleanup(func() { close(release); srv.Close() })
		c, err := New(Config{AppKey: testKey, AppSecret: testSecret, BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		start := time.Now()
		_, err = c.Send(context.Background(), Message{From: "oh_bot", ChannelID: "p17", Type: "OH:Queue", Content: "{}", Transient: true})
		if err == nil || time.Since(start) > 2*time.Second {
			t.Fatalf("Send returned %v after %v, want a timeout error within 2 s", err, time.Since(start))
		}
	})
}
