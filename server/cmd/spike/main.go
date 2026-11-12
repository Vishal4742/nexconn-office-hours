// Command spike is the P0 check of the Nexconn server calls the app depends on (ARCHITECTURE §8:
// A-2, A-4 server side, A-11). It runs them against the real app from .env and prints PASS or FAIL
// per step. It never prints the App Key, the App Secret or a token. Delete it when P0 closes.
//
//	set -a; . ./.env; set +a; go -C server run ./cmd/spike
//
// One argument (text, hand, session, faq, ping or forged) sends only that message, to test it alone.
package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"officehours/nexconn"
)

const (
	bot     = "oh_bot"
	userA   = "ohspikea"
	userB   = "ohspikeb"
	channel = "ohspike1" // fixed, so a rerun reuses the same test group
)

func main() {
	only := ""
	if len(os.Args) > 1 {
		only = os.Args[1]
		// A typo must not end in "all server steps passed" with nothing sent.
		if !slices.Contains([]string{"text", "hand", "session", "faq", "ping", "forged"}, only) {
			fmt.Println("FAIL unknown message:", only, "(use text, hand, session, faq, ping or forged)")
			os.Exit(1)
		}
	}

	client, err := nexconn.New(nexconn.Config{
		AppKey:    os.Getenv("NEXT_PUBLIC_NEXCONN_APP_KEY"),
		AppSecret: os.Getenv("NEXCONN_APP_SECRET"),
		BaseURL:   os.Getenv("NEXCONN_API_BASE_URL"),
		BackupURL: os.Getenv("NEXCONN_API_BACKUP_URL"),
	})
	if err != nil {
		fmt.Println("FAIL config:", err)
		os.Exit(1)
	}
	if os.Getenv("NEXCONN_API_BACKUP_URL") == "" {
		fmt.Println("note  NEXCONN_API_BACKUP_URL is empty: the primary domain is used for both")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	failed := 0
	step := func(name string, err error, detail string) {
		if err != nil {
			failed++
			fmt.Printf("FAIL  %-34s %v\n", name, err)
			return
		}
		fmt.Printf("PASS  %-34s %s\n", name, detail)
	}

	for _, u := range []struct{ id, name string }{{bot, "Office Hours"}, {userA, "Spike A"}, {userB, "Spike B"}} {
		token, err := client.IssueToken(ctx, u.id, u.name)
		step("issue token: "+u.id, err, fmt.Sprintf("token of %d chars", len(token)))
	}

	step("create group: "+channel, client.CreateGroup(ctx, channel, "Office Hours spike", bot), "owner "+bot+" (safe to repeat)")
	step("join group: 2 users", client.JoinGroup(ctx, channel, userA, userB), userA+", "+userB)

	send := func(name string, m nexconn.Message) {
		m.From, m.ChannelID = bot, channel
		id, err := client.Send(ctx, m)
		step(name, err, "message id "+id)
	}
	want := func(name string) bool { return only == "" || only == name }

	stamp := time.Now().Format("15:04:05")
	if want("text") {
		send("send text with @all", nexconn.Message{Type: "RC:TxtMsg", Mention: true,
			Content: fmt.Sprintf(`{"content":"@all spike run at %s","mentionedInfo":{"type":1,"userIdList":[]}}`, stamp)})
	}
	if want("hand") {
		send("send OH:Hand card (stored, push)", nexconn.Message{Type: "OH:Hand", Push: "Spike A raised a hand",
			Content: `{"handId":"spike-h1","sessionId":"spike-s1","title":"Does the card render?","authorLogin":"ohspikea"}`})
	}
	if want("session") {
		send("send OH:Session with @all", nexconn.Message{Type: "OH:Session", Push: "Office hours started", Mention: true,
			Content: `{"sessionId":"spike-s1","title":"Spike session","state":"live","mentionedInfo":{"type":1,"userIdList":[]}}`})
	}
	if want("faq") {
		send("send OH:FAQ card (stored)", nexconn.Message{Type: "OH:FAQ", Push: "Added to the FAQ",
			Content: `{"faqId":"spike-f1","title":"How do I run the tests?","slug":"how-do-i-run-the-tests"}`})
	}
	if want("ping") {
		send("send OH:Queue ping (transient)", nexconn.Message{Type: "OH:Queue", Transient: true,
			Content: fmt.Sprintf(`{"sessionId":"spike-s1","version":1,"sentAt":%d}`, time.Now().UnixMilli())})
	}
	if want("forged") {
		// FR-QUEUE-7: a card that does not come from the bot. Every client must refuse to act on it.
		forged, err := client.Send(ctx, nexconn.Message{From: userA, ChannelID: channel, Type: "OH:Hand",
			Content: `{"handId":"forged","sessionId":"spike-s1","title":"Forged card: must show as ignored","authorLogin":"ohspikea"}`})
		step("send forged OH:Hand as "+userA, err, "message id "+forged)
	}
	if only == "" {
		step("mute "+userB+" for 1 min", client.Mute(ctx, channel, userB, 1), "in "+channel)
	}

	if failed > 0 {
		fmt.Printf("\n%d step(s) failed\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nall server steps passed")
}
