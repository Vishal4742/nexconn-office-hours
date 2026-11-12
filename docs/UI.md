# UI plan — Office Hours

**v1 (2026-11-12).** Requirement IDs refer to `SRS.md`. Judging gives 50% to "the interface is clean, and the interaction is clear and easy to understand", so the UI optimises for one thing: **anyone understands the room in 5 seconds**: who is answering, who is next, where I am, and what I can do.

## 1. Direction v3: "one session, hour by hour" (2026-11-12)
**References are for principles, never for layouts** (owner, 2026-11-12: "do not do exact copy"). Two sources:
- **The owner's Pinterest board "refrence design"** (11 pins on 2026-11-12). What they share: an atmospheric image plus a deliberate graphic treatment over it (film grain, a dot-matrix overlay, RGB colour fringing, a painted brush-stroke reveal), small editorial labels, glass panels over imagery, and a sans headline with one italic serif word.
- **A mentor's reference, openvenue.xyz.** What we take: the story is told by scrolling, big confident numbers, and real product UI shown in context.
- **Two X posts from the owner (2026-11-12):** a tower whose window shutters open and close to draw a picture across the facade while the sky goes from pink dusk to amber night (Brice, three.js + WebGPU), and flowers drawn as a grid of glowing coloured dots on black (matthew hamilton, plain HTML). What they share: a picture made of many small units that each switch on, and light that carries the mood. What we take: both back the dot treatment we already have. The dots can carry the photo's colour and glow on the night frames, and they can switch on in a wave instead of all growing at once.

Not taken from the X posts: the 3D tower, the flowers, three.js or WebGPU (a heavy dependency for a page judged on running smoothly).

Not taken from the first two (these would be copies): a top announcement bar, a dark pill nav, a photo that shrinks while the headline splits, a ticker, a bento grid with dot charts, torn-paper edges, a diagonal brush window, poster lettering.

**Our own concept:** maintainers do this in their evenings, so the landing follows one session by the clock. The same scene (a desk, a lit lamp and an empty chair on a meadow) moves from afternoon to golden hour, dusk and night as you scroll, and each hour shows the real product UI for that moment: 17:00 doors open · 17:04 a hand goes up · 17:12 it's your turn · 17:31 answered once · 17:45 last call. **The room speaks the same language as the landing** (owner, 2026-11-12: "hero ok but room is still old design"): the room is the hero's composition made useful: the session as a headline on the left, and on the right a tall photo of the scene, lit by the session's clock, with the queue floating over its sky as glass.

- **Light theme**: paper background, ink text. The room is light only (Chat UI has no dark mode). Landing and FAQ pages also have a dark mode that follows the system; photos dim.
- **One accent: electric blue `#0047FF`** (Chat UI's own send-button blue, so the parts we can't restyle match): primary pill buttons, "now answering", my position, links, the selection box. Everything else is ink and greys. Nature colours appear only inside imagery. One exception: lamp gold `#F2C27A` for a single serif word on the night photo.
- **Type:** Instrument Sans (UI, 420/500) · Instrument Serif italic (one display phrase per page) · Geist Mono (queue numbers, timers, `owner/repo`, labels).
- **Imagery:** four AI-generated photos of the same scene (Figma AI, in the Figma file "Office Hours imagery"), in `../design/assets/`: `hero-meadow.webp` (afternoon), `golden.webp`, `dusk.webp`, `night.webp`, plus `night-ca.webp` (the night photo with a 3 px RGB channel split, made with ImageMagick). 59–92 KB each. Copy these five into `web/public/` in P1. The misty sky bands (tagline, FAQ header) are drawn in SVG.
- **Room imagery:** four more photos of the same scene in portrait (864 × 1536: open sky on top, the desk about three quarters down, meadow at the bottom), made for the room's queue panel: `panel-day.webp`, `panel-golden.webp`, `panel-dusk.webp`, `panel-night.webp` (42–79 KB each). Copy these into `web/public/` too. Eight Figma AI generations in total, all first tries.
- **Three treatments, one per section, each with a reason:**
  1. **Hero: painted reveal.** The afternoon photo shows through a brush-painted swath on the paper (an SVG mask: 75 thin bristle rows with ragged ends, slightly roughened). Glass queue cards overlap its edge.
  2. **Story: dots.** The session clock is a 5×7 dot-matrix display. When the hour changes, the next photo switches on dot by dot in a wave that spreads out from the lamp (drawn on a canvas by `dotWave`, 1.4 s), then the picture is whole again. On the dusk and night frames the last photo dims first and the dots carry a lamp-gold glow, like windows lighting up. With reduced motion the photo just swaps. Dots stand for the live moment being recorded.
  3. **Closing: after hours.** The night photo with RGB fringing and light grain, a slight colour fringe on the headline, and "Thursday." in lamp gold.
- **Details:** "once." sits in a blue selection box with handles. **No decorative labels on the landing** (owner, 2026-11-12): no issue-number strip in the header (`Nº 018 · … · live now`), no figure captions (`fig. 01 · 17:00`), no section counters (`[ 001 / 003 ]`, `( 01 )`), no corner credits, and no mock queue cards floating over the hero photo. The photo stands alone. Mono labels stay where they carry real state: inside the room (`Now answering`, `Waiting [ 3 in line ]`), on the FAQ header (repo and answer count) and the story's clock.
- **Surfaces:** flat cards with a shadow ring; one glass style for sheets, the "your turn" banner and cards over photos. Pill buttons. Radius 12 (cards) / 24 (photo frames) / 999 (buttons).
- **Motion (off with reduced motion):** queue items slide to their new place (FLIP, 250 ms); "It's your turn" pulses twice and stops; the live dot breathes; on the landing the story frame stays pinned while steps scroll, photos change with the dot reveal, and clock dots flip.

### Tokens
| Token | Light | Dark |
|---|---|---|
| `--paper` / `--ink` | `#F7F6F2` / `#14151A` | `#0E0F13` / `#ECEBE6` |
| `--muted` / `--hairline` | `#6B6D76` / `#E3E1DA` | `#9A9CA6` / `#26282F` |
| `--blue` (accent) / `--blue-ink` (text on blue) | `#0047FF` / `#FFFFFF` | `#7A9BFF` / `#0E0F13` (landing + FAQ only) |
| status `waiting` / `answering` / `answered` / `skipped` / `not_reached` | grey / blue / `#1A7A43` / `#8F5E12` / muted strike | same hues, lighter |
| status `carried` / `merged` / `withdrawn` / `removed` | grey outline "Next session" / grey "Merged" / muted strike / muted strike | same, lighter |
All text pairs are checked to 4.5:1 (NFR-A11Y-1) in P4.

## 2. Screens
| # | Route | Purpose | SRS | Built in |
|---|---|---|---|---|
| S1 | `/` | Landing: explain in 20 s, sign in, "host office hours for your repo", live sessions now | FR-AUTH-1 | P1 (art P4) |
| S2 | `/dashboard` | My projects + joined rooms; register a repo | FR-PROJ-1..2 | P1 |
| S3 | `/p/{owner}/{repo}` | **The room**: join gate, chat, queue, session controls | FR-PROJ-3..4, SESS, HAND, QUEUE | P1 shell → P2 queue |
| S4 | `/p/{owner}/{repo}/faq` | FAQ list + search (public) | FR-FAQ-3 | P3 |
| S5 | `/p/{owner}/{repo}/faq/{slug}` | One answer (public, link previews) | FR-FAQ-3,5 | P3 |
| S6 | `/p/{owner}/{repo}/stats`, `/stats` | Big numbers for traction | FR-MET-1 | P4 |

### S3 — the room (two columns at ≥ 1040 px)
The first version (three plain columns) and the second (a thin painted strip in the header) were both rejected by the owner ("room is still old design", "rooms not looking good"): the scene was decoration on top of an unchanged layout. This version makes the picture the queue's own surface.
```
 ◖ Office Hours / paperplane/stash ● LIVE   [Last call][End session] 🔗 [FAQ](Me)  ┌ photo panel, 400 px, radius 24 ──┐
 Thursday office hours, live.          (36 px, "live." in blue serif)              │ [⣿ 23:47 golden hour]  [18 here]  │
 Nº 018 · 45 min with @mia-k · 22 min left                                         │ ┌ NOW ANSWERING (blue) ─────────┐ │
┌ white card, radius 20 (Nexconn Chat UI) ──────────────────────────────────────┐  │ │ (DR) @diego-ruiz              │ │
│ Office Hours  10:10                                                           │  │ │ How do I retry only on 5xx…?  │ │
│  ┌ bubble: FAQ card ────────────────────────────────────┐                     │  │ └───────────────────────────────┘ │
│  │ 📌 ADDED TO FAQ · How do I revalidate on focus…  ↗    │                     │  │ ⌜ ★ 3 ahead of you            ⌟ │
│  └───────────────────────────────────────────────────────┘                     │  │ ┌ glass: WAITING [ 3 in line ] ─┐ │
│ Mia Kowalski [maintainer] 10:20                                               │  │ │ 01 @ana-ferreira …       [+1] │ │
│  ┌ bubble ─────────────────────────────┐                                      │  │ │ 02 @liwen …              [+2] │ │
│  │ @diego-ruiz you're up. Retries on…  │                                      │  │ │ Done this session        1 ⌄  │ │
│  └─────────────────────────────────────┘                                      │  │ └───────────────────────────────┘ │
│                                                                               │  │      sky → desk + lamp + chair     │
│ [ 🙂 📎  Message the room                                          ( Send ) ] │  │ ┌ glass ────────────────────────┐ │
└───────────────────────────────────────────────────────────────────────────────┘  │ │    [ ✋ Raise your hand ]       │ │
                                                                                   └─┴───────────────────────────────┴─┘
```
- **Left column: headline, then the chat.** A brand row (logo, repo, live label; on the right the maintainer's Last call and End session, copy room link, FAQ, my avatar). The session as a 36 px headline whose last word is the status in blue italic serif ("…, *live.*", "…, *ended.*", "No session *right now.*"). A mono line: `Nº`, length, host, minutes left. Below it Chat UI in a white card (radius 20, our bubble colours and radius 12, minimum width 600).
- **Right column: the photo panel (400 px, full height, radius 24).** A portrait photo of the scene fills it (`object-fit: cover`, anchored to the bottom). The queue floats over the sky:
  - **Top row:** a glass chip with the dot-matrix clock (elapsed `MM:SS`, `H:MM` from 100 minutes) and the hour name ("golden hour", "last call"), and a chip with avatars and "18 here".
  - **Now answering:** the one solid blue card (a faint dot texture in a corner). Maintainers get the question timer, "here" or "waiting for a reply", and Done / Skip or Away / issue.
  - **My place:** a selection box with two handles (the same motif as "once." on the landing): "3 ahead of you", the wait estimate, Withdraw, Notify me.
  - **Waiting:** one glass card with numbered rows (+1 for contributors, a ⋯ menu for maintainers) and "Done this session" at its foot. If the list is longer than the space, the card scrolls and fades out at the bottom.
  - **A window of at least 104 px** is always left free above the button, so the desk and lamp stay visible.
  - **The main action** sits in a glass card at the bottom: Raise your hand (contributor), Next question `N` (maintainer), Start a session (maintainer, not live).
- **The light follows the session.** Afternoon photo for the first 35% of the planned length, golden hour to 70%, dusk to 95%, then night; Last call and an ended session are night; no session is afternoon. Computed in the browser from `started_at` and `planned_minutes` (45 if not set): no server work. Photos cross-fade over 1.6 s; each is scaled a little (1.04 / 1.12 / 1.11 / 1.10) so the desk lands in the same place. Glass gets more opaque at dusk (0.82) and night (0.88) so text keeps its contrast.
- **Ending needs a confirmation sheet** (End this session? + the carry-over checkbox), because End now sits in the header.
- **Not live:** the panel shows a glass card "No session right now · Next session: Thu 17:00 (your time)" (FR-SESS-5), answer search ("While you wait"), and Raise hand disabled with "Opens when the session is live".
- **Signed out / not joined:** the landscape photo behind a glass join card: repo, the session, "Sign in with GitHub to join", latest answers (FR-PROJ-4).
- **Keyboard (maintainer):** `N` Next, `D` Done, `S` Skip, `A` Away, with visible hints (NFR-A11Y-1).
- Project description and recent answers are no longer a desktop column: the FAQ button opens the FAQ page; on phones they live in the FAQ tab.

### S3 on phones and tablets (below 1040 px): tabs, queue first (NFR-COMP-1)
```
┌─────────────────────────────┐
│ ◖ paperplane/stash  ● LIVE (Me)│
│ [ Queue 4 ] [ Chat ] [ FAQ ]   │
├─ photo fills the tab ─────────┤
│ [⣿ 23:46 golden hour] [18 here]│
│ ┌ NOW ANSWERING ────────────┐  │
│ ⌜ ★ 3 ahead of you         ⌟  │
│ ┌ WAITING [ 4 in line ] ───┐  │
│ │ 01 @ana …           [+1] │  │  ← fades out if longer
│      desk + lamp + chair       │
│ ┌ [   ✋ Raise your hand   ] ┐  │  ← maintainers: Next ▸, then Last call / End session
└─────────────────────────────┘
```

### Sheets and states inside the room
- **Raise-hand sheet** (FR-HAND-1,3): title, then details. Under the title, "Already answered?" shows up to 3 FAQ matches as you type (debounced 300 ms). Each opens in a side preview, "That solved it" closes the sheet, and "Raise my hand" submits. Counters show the character limits.
- **Done sheet** (FR-FAQ-1): the question is prefilled; "Answer summary" (20–2,000) + links; a live preview of the FAQ card; "Publish to FAQ".
- **"It's your turn"** (FR-QUEUE-5): a blue glass banner across the room ("@mia is answering your question"), the queue row turns blue, an assertive `aria-live` announcement, an optional browser notification (FR-NOTIF-2) and one gentle sound if the tab is hidden.
- **Session ended:** a summary card: "12 questions · 9 answered · 7 new FAQ entries · see you Thursday".
- **Connection:** a slim banner "Reconnecting…" when Nexconn drops (FR-AUTH-3); the queue refetches on reconnect (FR-QUEUE-6).
- **Errors:** inline and specific ("You already have an open hand", "Wait 40 s before raising another"); never a silent failure.

### S1 landing
```
 Office Hours                                                                Sign in  [ Host office hours ]
 ──────────────────────────────────────────────────────────────────────────────────────────────────────────
  Answer every question                         ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ afternoon photo, shown through
  ⌜once.⌟  (blue selection box)                 ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ a painted swath on the paper,
  Live office hours for your open source…       ▓▓▓▓▓ desk + lamp + chair ▓▓▓ nothing on top of it
  [ Host office hours for your repo ]  See a live room →
 ──────────────────────────────────────────────────────────────────────────────────────────────────────────
  Forty five minutes, one question at a time.
  17:00 Doors open      + session card          ┌──────────────────────────────┐  pinned photo frame:
  17:04 A hand goes up  + suggestion            │  afternoon → golden → dusk   │  changes with a dot reveal
  17:12 It's your turn  + banner                │  → night                     │
  17:31 Answered once   + FAQ card              │  ⣿⣿ 17:12 (dot-matrix clock) │
  17:45 Last call       + ended card            └──────────────────────────────┘
 ──────────────────────────────────────────────────────────────────────────────────────────────────────────
  Made for people who maintain in their evenings:  ≤1s · 1× · 0 (big serif numbers) + three short benefits
  misty sky band: "The same question should never cost you two evenings…" (words light up on scroll)
  Before you host: 7 questions, plain question and answer
  night photo: "See you Thursday." + both buttons            footer
```
One primary action (Host office hours for your repo), repeated in the closing. On phones each story step carries its own photo and the pinned frame is hidden.

### S4–S5 FAQ (public, server-rendered)
A search field on top; entries as cards (question, the first line of the answer, date, maintainer avatar). The entry page: question as the title, answer, links, "Answered live in office hours on 18 Nov by @mia", and at the bottom "Join the next session → Thu 17:00" + "Open a GitHub issue" (FR-FAQ-5). Link-preview metadata so a pasted link shows the question.

## 3. Chat UI integration
- Mount only Chat UI's channel-detail region (message list + input). Hide the channel list and its bar with the layout switches; our header replaces them (`chatui-web/chat-ui/layout`).
- Our 3 custom bubbles (`OH:Session`, `OH:Hand`, `OH:FAQ`) are our own web components, styled with our tokens; `OH:Hand` reads its live status from the queue store.
- Bubble radius set to match ours (`MessageBubbleConfig`); en_US language pack; input toolbar trimmed to text, emoji, image, file.
- **A-9 (check in P0):** can Chat UI's colours be set through CSS custom properties? If yes, map them to our tokens. If not, keep our shell neutral around it so the two looks don't clash.

## 4. Components (React unless noted)
`AppShell` · `RepoHeader` (live dot, timer, participants) · `ChatRoom` (Chat UI singleton) · `QueuePanel` · `QueueItem` · `NowAnswering` · `RaiseHandSheet` + `FaqSuggestions` · `DoneSheet` · `YourTurnBanner` · `SessionControls` · `SessionSummary` · `ConnectionBanner` · `FaqList` · `FaqEntry` · `StatsTiles` · `EmptyState` (with art) · `SignInButton` · web components `oh-session-card`, `oh-hand-card`, `oh-faq-card`.

## 5. Every view has all four states
Loading (skeleton rows, never a spinner over the whole room) · empty (art + one sentence + one action) · error (what happened + retry) · success. Checked per view in P4 (AT-13, AT-14).

## 6. Process
1. **References (owner picks, 10 min):** Mercury (the calm the owner already chose), Linear (dense but quiet lists), Raycast (keyboard-first), GitHub Discussions (familiar to OSS users), Discord stage channels (the live-room feeling), Cal.com (clear scheduling). Pick what to borrow from each.
2. **Clickable local demo:** one HTML file, `~/nexconn-build-challenge-2026/design/office-hours-demo.html` (outside the public repo), with demo data. It shows: landing, the room (maintainer and contributor views, with a fake live queue moving), raise-hand sheet with suggestions, "It's your turn", Done → FAQ card, the FAQ page, and the phone layout. **The owner approves it before P1 UI work starts.** ✅ Approved 2026-11-12 (demo v6: the hero, then the rebuilt room).
3. **Build per phase:** P1 shell, header, join gate, landing (without art) · P2 queue panel, sheets, your-turn, keyboard · P3 FAQ pages, Done sheet, cards · P4 imagery, dark mode (landing + FAQ pages only), motion, states, phone polish, stats.
4. **Demo video check (P6):** the room must read at 1080p. Queue numbers and "now answering" are big enough to read in a compressed video.

## 7. Research, 2026-11-12 (applied in the demo; the owner approves before P1)
### Chat UI facts that shape the design (A-9 answered: ❌, no theme tokens)
Sources: `docs.nexconn.ai/chatui-web/*.md`, `github.com/NexconnAI-Dev/nexconn-chatui-web` (src), `unpkg.com/@nexconn/chatui@26.2.5` (bundle + `index.d.ts`).
- The chat has no CSS custom properties (only 3 channel-list vars). Open shadow DOM, no `::part`. Hard-coded: white message list, header, input box, send button (`#0047FF` on `#ECF1FF`). **No dark mode.** The font family inherits from the host; sizes are fixed (12/14/16).
- Bubbles: `app.setMessageBubbleConfig({ ...app.cloneMessageBubbleConfig(), redius, layout, backgroundColorForMyself, backgroundColorForOthers, textColorForMyself, textColorForOthers })` before `ready()`. Colours are hex integers (no alpha). Avatars are 32 px, every bubble has its own name/time line, and there is **no grouping**.
- Custom bubbles: `registerCustomMessages([{ messageType, isPersisted, isCounted, digest, component: { tagName, elementClass } }])` → tag `nc-c-<tagName>`, receives `.message`. It sits inside `nc-message-bubble` (bubble colour + radius, `max-width: 70%`, overflow hidden). Style it with Lit `static styles`, since page CSS doesn't reach it.
- `nc-channel-detail-provider` always draws its own 70 px header (no switch), and on a phone that header gets a back arrow that leads to a channel list we don't have. **Confirmed in the P0 spike (2026-11-12): the room mounts `nc-toast-provider` + `nc-message-list-provider` + `nc-input-provider` itself**, after `app.openChannel()`, with `.model = app.getOpenedChannel()` on the input. No header, and the room draws its own. Don't use `setChannelBarSwitch(false)`: it only hides the list's title bar, and only at ≤ 768 px.
- The channel detail has `min-width: 600px` (350 px in mobile mode). Mounted as parts there is **no minimum width**: the list and input fit a 390 px viewport with no sideways scroll. Mobile mode turns on when the **viewport** is ≤ 768 px: the input loses its placeholder and the send button becomes an icon. Supported browsers: Chrome 68+, Safari 13+, Edge 100+. Portrait mobile is not a supported target, so a real phone is still to try.
- Sender names and avatars are copied when the room opens and are not refreshed (`ARCHITECTURE.md` §4): the room must hand Chat UI its members before it opens, or people show as raw ids. My own messages show a time and no name.
- A muted member's message stays in the list with a small red "!" and no reason. The room should say "You are muted until 14:05" above the input and disable it, since our server knows.
- The first connection of a visit takes 1.2 to 1.5 s (5 s once) and history arrives a moment later: the chat card needs a loading state.
- Input menu: `cloneInputMenu()` → edit `items` → `setInputMenu()`. Ids: `EMOJI | IMAGES | FILES | PHOTO | PLUS`. Language `en_US` is the default; `registerLanguagePack` rewrites any string.
- Message menu (right click, or the "⋯" on hover): `cloneMessageMenu()` → filter → `setMessageMenu()`, before `ready()`. Ids: `REPLY | MULIT_CHOICE | COPY | FORWARD | SPEECH_TO_TEXT | CANCEL_SPEECH_TO_TEXT | DELETE_FOR_ME | DELETE_FOR_ALL`. The room keeps Reply, Copy and the two deletes.
- **Tried between two users in the real room (2026-11-12), all work:** text, an @mention (typing `@` lists "@all", the bot and the other members), a reply, read receipts ("Read: 1, Unread: 1" on my own message, a green mark once someone read it), an image and a file (both upload to Nexconn's storage; an image is sent at once, with no confirm step). An image bubble shows a square crop of the picture.
- A reply bubble prints the quoted author's **raw user id**. We replace it with the name (`web/src/nexconn/replyNames.ts`); the composer's own "Reply X:" line was always right.
- Someone who joins a room sees **no earlier messages** unless the Console switch "Enable pre-join history access by new member" is on (Chat → Chat settings → Group Channels). The owner should turn it on: a person who joins in the middle of a session should see how it started.
- The first connection from a browser can time out once (code 35008 after two tries of 10 s and 5 s). The banner's "Try again" connected at once. Connecting took between 1.3 s and 6 s on this day, so the room says "Opening the room…" until the list is in.

### Decisions this forces (owner-approved 2026-11-12)
1. Accent `#1723E2` → **`#0047FF`**, Chat UI's own send-button blue, so the parts we can't restyle match ours. Wash `#EEF2FF`.
2. **The room is light only** (Chat UI has none, and a white chat inside a dark shell looks broken). Landing and FAQ pages also get a dark mode that follows the system (owner: "both").
3. The chat pane is a white surface (Chat UI's). Side panels stay paper, with `#C8C4B8` dividers.
4. Bubbles: `redius: 12`, `LEFT_JUSTIFYING`, others `0xF1F0EC`, mine `0xE6EDFF`, text `0x14151A`. OH cards are designed to live inside a bubble, with no border or background of their own.
5. Two columns at ≥ 1040 px (chat ≥ 600 px + photo queue panel 400 px + 12 px gaps) and tabs below. The 600 px came from the channel detail's minimum, which no longer binds now that the room mounts the parts; it stays as the width where the chat reads well.

### Queue patterns
Sources:
- Illinois Queue issues #341, #302, #327 and #10
- Michigan office-hours-queue issues #18, #22, #40 and #48
- Waitwhile, Intercom and Google Meet
- NextUP, the office-hours queue that won "Best UI/UX" at UB Hacking 2024

**Applied in the demo:**
- "N ahead of you". It never counts the person being answered.
- A wait estimate once there are 2 answers, from this session's median.
- The place in the tab title: "(2 ahead) Office Hours".
- "I'm here" on the your-turn banner. The maintainer sees "here" or "waiting for a reply".
- A timer on the current question, shown only to the maintainer, that turns amber after 8 min.
- One gentle chime if the tab is hidden.
- "Notify me" sends a test notification.
- The maintainer is pinged only when the queue goes from empty to not empty.

**Owner-approved 2026-11-12, now Must in SRS v1.2** (FR-QUEUE-8..9, FR-SESS-6..7, FR-HAND-7..8; built in the demo):
- Away: after a missed turn, the person moves back 2 places instead of being skipped.
- "Last call" closes hands before the end.
- Not-reached hands carry over to the next session.
- "+1 me too", and merging duplicate questions.
- A 5 s undo after N/D/S.

**Pitfalls to avoid:**
- Never sort the turn queue by votes.
- Never rely on browser notifications alone.
- Show one order, from the server, everywhere.
- Never remove someone silently.

### Visual values
Sources: vercel.com/geist, linear.app/now, mercury.com, cal.com. Dither examples: Replicate, Mintlify, E2B, Amp, and surma.dev/things/ditherpunk.

**Applied in the demo:**
- Heading tracking: −0.012em up to 24 px, −0.022em from 32 px.
- Cards on white use `box-shadow: 0 0 0 1px rgba(20,21,26,.08)` instead of borders.
- Motion: hover 150 ms, queue rows 250 ms, sheets 400 ms with `cubic-bezier(.32,.72,0,1)`.
- The now-answering avatar is 40 px with a 2 px ring.
- Art: superseded by §1 v3 (photos with three treatments). The dithered illustration from v1 is gone.

**For the video (P6):** record at 1920×1080 with 125% zoom. Keep anything the viewer must read at 13 px CSS or larger. Queue panel at least 340 px.

## 8. Demo v7 (2026-11-12): v6 approved by the owner, then cleaned up at their request
**v7, 12:26 IST:** the owner asked to remove the mock queue cards over the hero photo, the header's issue-number strip, the figure captions "and others of these types". All decorative mono labels are gone from the landing (§1 Details has the list). Nothing else changed; the room, the FAQ pages and the story's time labels and dot clock are as approved. The v6 file is kept as `../design/.work/demo-v6-approved.html`.

`../design/office-hours-demo.html` with `../design/assets/` beside it (outside the repo; open it in Chrome). The bar at the bottom switches screen, role, device, state and dark mode; **Jump to** opens key moments. `#selftest` runs 31 checks of the queue rules (all pass). The landing follows §1 v3 (hero approved by the owner); the room follows §2 S3 (photo queue panel) with Chat UI's real constraints (§7); the queue has the approved extras: +1 on waiting rows, Away while there's no reply, Last call, carry-over on end, "N ahead of you", a wait estimate, the place in the tab title.
