package main

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"officehours/nexconn"
	"officehours/queue"
	"officehours/store"
)

// What the server says in a room. Every message is sent by the bot (botID), and the browser shows
// an OH:* message only when the bot sent it (FR-QUEUE-7). The payloads below are a contract with
// web/src/nexconn/cards.ts: ids are strings, and text is plain text, never HTML.

// chatService is the part of the Nexconn client the queue talks to a room through.
type chatService interface {
	Send(ctx context.Context, m nexconn.Message) (string, error)
	Mute(ctx context.Context, channelID, userID string, minutes int32) error
}

// The message types (ARCHITECTURE §4). The browser registers the same names.
const (
	typeSession = "OH:Session"
	typeHand    = "OH:Hand"
	typeFAQ     = "OH:FAQ"
	typeQueue   = "OH:Queue"
	typeText    = "RC:TxtMsg"
)

// tellTimeout bounds one round of messages. Nexconn's own calls give up after 3 s.
const tellTimeout = 5 * time.Second

// tell sends messages into a project's room as the bot, all at once, and waits for them. It never
// fails the request: by now the change is written, so a send that fails is logged, and every
// client reads the queue again on focus, on reconnect and on the next ping (ARCHITECTURE §6).
// The sends outlive a caller who has gone away: a change that was written must be announced.
func (d deps) tell(ctx context.Context, projectID string, msgs ...nexconn.Message) {
	if d.chat == nil || len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tellTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, m := range msgs {
		m.From, m.ChannelID = botID, groupID(projectID)
		wg.Go(func() {
			if _, err := d.chat.Send(ctx, m); err != nil {
				log.Printf("tell the room (%s): %v", m.Type, err)
			}
		})
	}
	wg.Wait()
}

// payload is a message's content as Nexconn wants it: a JSON object inside a string. Everything
// goes through json.Marshal, so a title can never break out of its field.
func payload(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("message payload: %v", err)
		return "{}"
	}
	return string(b)
}

func idText(id int64) string { return strconv.FormatInt(id, 10) }

// queuePing tells the room that a session's queue has a new version. It is a hint and never
// state: it is not stored, shows nothing and counts as no unread message, and a client that holds
// this version or a newer one ignores it (FR-QUEUE-4). sentAt lets a client log how long the
// change took to arrive (NFR-PERF-1).
func queuePing(sessionID int64, version int) nexconn.Message {
	return nexconn.Message{Type: typeQueue, Transient: true, Content: payload(struct {
		SessionID string `json:"sessionId"`
		Version   int    `json:"version"`
		SentAt    int64  `json:"sentAt"` // Unix milliseconds
	}{idText(sessionID), version, time.Now().UnixMilli()})}
}

// mentionAll is how a message says "@all" (confirmed in the P0 spike, assumption A-12).
type mentionAll struct {
	Type       int      `json:"type"` // 1: everyone in the room
	UserIDList []string `json:"userIdList"`
}

// sessionCardContent is what an OH:Session card carries.
type sessionCardContent struct {
	SessionID     string      `json:"sessionId"`
	Title         string      `json:"title"`
	State         string      `json:"state"` // "live" or "ended"
	Answered      int         `json:"answered"`
	Carried       int         `json:"carried"`
	NotReached    int         `json:"notReached"`
	MentionedInfo *mentionAll `json:"mentionedInfo,omitempty"`
}

// sessionStarted is the card that opens a session. It mentions everyone, so members get an unread
// badge and, where they allow it, a push (FR-SESS-3).
func sessionStarted(s store.Session) nexconn.Message {
	return nexconn.Message{Type: typeSession, Mention: true, Push: "Office hours started: " + s.Title,
		Content: payload(sessionCardContent{SessionID: idText(s.ID), Title: s.Title, State: "live",
			MentionedInfo: &mentionAll{Type: 1, UserIDList: []string{}}})}
}

// sessionEnded is the card that closes a session, with what became of its hands (FR-SESS-4,
// FR-SESS-7).
func sessionEnded(sessionID int64, title string, answered, carried, notReached int) nexconn.Message {
	return nexconn.Message{Type: typeSession, Push: "Office hours ended: " + title,
		Content: payload(sessionCardContent{SessionID: idText(sessionID), Title: title, State: "ended",
			Answered: answered, Carried: carried, NotReached: notReached})}
}

// handRaised is the card of a new hand (FR-HAND-5). A card cannot change once it is sent, so the
// browser reads the hand's current status from the queue by handId.
func handRaised(sessionID int64, h queue.Hand, authorLogin string) nexconn.Message {
	return nexconn.Message{Type: typeHand, Push: "@" + authorLogin + " raised a hand: " + h.Title,
		Content: payload(struct {
			HandID      string `json:"handId"`
			SessionID   string `json:"sessionId"`
			Title       string `json:"title"`
			AuthorLogin string `json:"authorLogin"`
		}{idText(h.ID), idText(sessionID), h.Title, authorLogin})}
}

// faqAdded is the card of a new FAQ entry (FR-FAQ-2). It carries the slug and no address: the
// browser builds the link on our own origin (NFR-SEC-6). askedBy are the logins of everyone who
// asked, the author first (FR-HAND-8); the public page shows only how many they are.
func faqAdded(e store.FAQEntry, askedBy []string) nexconn.Message {
	if askedBy == nil {
		askedBy = []string{}
	}
	return nexconn.Message{Type: typeFAQ, Push: "Added to the FAQ: " + e.Title,
		Content: payload(struct {
			FAQID   string   `json:"faqId"`
			Slug    string   `json:"slug"`
			Title   string   `json:"title"`
			AskedBy []string `json:"askedBy"`
		}{idText(e.ID), e.Slug, e.Title, askedBy})}
}

// botSays is a plain line from the bot, for what the room must be told in words (FR-QUEUE-8:
// why someone moved back).
func botSays(text string) nexconn.Message {
	return nexconn.Message{Type: typeText, Content: payload(struct {
		Content string `json:"content"`
	}{text})}
}
