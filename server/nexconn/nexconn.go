// Package nexconn is the only place that talks to Nexconn's server API (signed REST, through the
// official Go SDK). The App Secret stays inside this package: it is never logged or put in an error.
package nexconn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	ncsdk "github.com/NexconnAI-Dev/nexconn-server-sdk-go"
)

// Config is what the server needs to call Nexconn.
type Config struct {
	AppKey    string
	AppSecret string
	BaseURL   string        // the data center's API domain, e.g. https://api.sg-light-api.com
	BackupURL string        // optional second domain for failover; BaseURL is used when empty
	Timeout   time.Duration // per request; 3 s when zero
}

// Client calls Nexconn on behalf of the server.
type Client struct {
	api *ncsdk.APIClient
}

// Error is a refusal from the Nexconn API: an HTTP error, or a non-zero code in a 200 reply.
type Error struct {
	Op     string // what we were doing, e.g. "send message"
	Status int    // HTTP status
	Code   int    // Nexconn's business code; 0 means success
	Msg    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("nexconn %s: http %d, code %d: %s", e.Op, e.Status, e.Code, e.Msg)
}

const defaultTimeout = 3 * time.Second

// New builds a client. Without a timeout of its own the SDK would wait forever on a stuck call.
func New(cfg Config) (*Client, error) {
	if cfg.AppKey == "" || cfg.AppSecret == "" || cfg.BaseURL == "" {
		return nil, errors.New("nexconn: app key, app secret and base URL are all required")
	}
	backup := cfg.BackupURL
	if backup == "" {
		backup = cfg.BaseURL
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	sdk := ncsdk.NewConfiguration()
	sdk.SetNexconnCredentials(cfg.AppKey, cfg.AppSecret)
	if err := sdk.SetPrimaryBackupDomains(cfg.BaseURL, backup); err != nil {
		return nil, fmt.Errorf("nexconn: %w", err)
	}
	// Nexconn asks clients to keep idle connections for under 55 s.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.IdleConnTimeout = 50 * time.Second
	sdk.HTTPClient = &http.Client{Timeout: timeout, Transport: transport}
	return &Client{api: ncsdk.NewAPIClient(sdk)}, nil
}

// check turns the SDK's result into nil or an error. code is the business code of a decoded reply
// (the SDK's GetCode answers 0 for a missing reply).
func check(op string, code int32, resp *http.Response, err error) error {
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if err != nil {
		var apiErr *ncsdk.GenericOpenAPIError
		if errors.As(err, &apiErr) {
			return &Error{Op: op, Status: status, Code: apiErr.ErrorCode(), Msg: apiErr.ErrorMessage()}
		}
		return fmt.Errorf("nexconn %s: %w", op, err)
	}
	if code != 0 {
		return &Error{Op: op, Status: status, Code: int(code), Msg: "request refused"}
	}
	return nil
}

// IssueToken registers the user with Nexconn if needed and returns the access token the browser
// connects with (FR-AUTH-2).
func (c *Client) IssueToken(ctx context.Context, userID, name string) (string, error) {
	const op = "issue token"
	res, resp, err := c.api.UserManagementAPI.IssueAccessToken(ctx).
		AccessTokenIssueRequest(*ncsdk.NewAccessTokenIssueRequest(userID, name)).Execute()
	if err := check(op, res.GetCode(), resp, err); err != nil {
		return "", err
	}
	if res == nil || res.Result == nil || res.Result.AccessToken == nil || *res.Result.AccessToken == "" {
		return "", fmt.Errorf("nexconn %s: the reply has no access token", op)
	}
	return *res.Result.AccessToken, nil
}

// codeGroupExists is what Nexconn answers (with HTTP 400) when the channel id is taken.
const codeGroupExists = 25409

// CreateGroup creates a group channel, or does nothing if it already exists, so registering a
// project can be retried safely (FR-PROJ-2). The owner joins automatically. Channel ids may only
// contain letters and digits (max 64).
//
// "Already exists" is trusted without asking who owns it, so the id must be one no client could
// guess before we create the group: any signed-in user may be able to create a group from the
// browser SDK, and a guessable id (p1, p2, …) would let them own a project's room.
func (c *Client) CreateGroup(ctx context.Context, channelID, name, ownerID string) error {
	const op = "create group"
	res, resp, err := c.api.GroupChannelManagementAPI.CreateGroupChannel(ctx).
		GroupChannelCreateRequest(*ncsdk.NewGroupChannelCreateRequest(channelID, name, ownerID)).Execute()
	err = check(op, res.GetCode(), resp, err)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Code == codeGroupExists {
		return nil
	}
	return err
}

// JoinGroup adds users to a group channel (at most 100 per call).
func (c *Client) JoinGroup(ctx context.Context, channelID string, userIDs ...string) error {
	const op = "join group"
	res, resp, err := c.api.GroupChannelManagementAPI.JoinGroupChannel(ctx).
		GroupChannelJoinRequest(*ncsdk.NewGroupChannelJoinRequest(channelID, userIDs)).Execute()
	return check(op, res.GetCode(), resp, err)
}

// Message is one server-sent group message.
type Message struct {
	From      string // sender user id (our bot)
	ChannelID string
	Type      string // "RC:TxtMsg" or one of our custom "OH:*" types (registered in the browser)
	Content   string // the payload as a string: a JSON object for built-in types
	Push      string // push text for offline members; a custom type gets no push without it
	Mention   bool   // set when Content carries mentionedInfo
	Transient bool   // not stored in cloud history, no push, no last-message update (the queue ping)
}

// Send delivers one message to one group channel and returns Nexconn's message id.
func (c *Client) Send(ctx context.Context, m Message) (string, error) {
	const op = "send message"
	req := ncsdk.NewGroupChannelMessageSendRequest(m.From, []string{m.ChannelID}, m.Type, m.Content)
	persist := int32(1)
	if m.Transient {
		persist = 0
		yes := true
		req.DisablePush = &yes
		req.DisableUpdateLastMsg = &yes
	}
	req.ShouldPersist = &persist // a pointer, so 0 is sent and not dropped as "empty"
	if m.Push != "" && !m.Transient {
		req.PushContent = &m.Push
	}
	if m.Mention {
		one := int32(1)
		req.HasMention = &one
	}

	res, resp, err := c.api.MessageManagementAPI.SendGroupChannelMessage(ctx).
		GroupChannelMessageSendRequest(*req).Execute()
	if err := check(op, res.GetCode(), resp, err); err != nil {
		return "", err
	}
	if res == nil || res.Result == nil || len(res.Result.MessageIds) == 0 || res.Result.MessageIds[0].MessageId == nil {
		return "", nil // delivered, but Nexconn returned no id
	}
	return *res.Result.MessageIds[0].MessageId, nil
}

// Mute stops a member from posting in a group channel for the given number of minutes (FR-MOD-1).
func (c *Client) Mute(ctx context.Context, channelID, userID string, minutes int32) error {
	const op = "mute"
	req := ncsdk.NewGroupChannelUserMuteListAddRequest([]string{userID}, minutes)
	req.ChannelId = &channelID
	res, resp, err := c.api.GroupChannelModerationAPI.AddGroupChannelUserMuteList(ctx).
		GroupChannelUserMuteListAddRequest(*req).Execute()
	return check(op, res.GetCode(), resp, err)
}
