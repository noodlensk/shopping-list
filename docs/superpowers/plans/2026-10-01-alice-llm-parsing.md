# Alice LLM Parsing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Alice echo spike with a handler that sends each spoken phrase to an LLM, adds the returned items to the shopping list, and tells the user what was added.

**Architecture:** A new OpenRouter adapter turns a phrase into `{intent, items, done}` using a strict JSON schema. The Alice port calls it synchronously with a 2.5 s timeout, adds items through the existing `Commands.AddItem`, and falls back to saving the raw phrase when the LLM fails. Config gets an `OpenRouter` section; `main.go` wires it.

**Tech Stack:** Go 1.25, `github.com/OpenRouterTeam/go-sdk v0.7.25` (same as `../swedish-news`), BoltDB, zap, viper.

**Spec:** `docs/superpowers/specs/2026-10-01-alice-llm-parsing-design.md`

## Global Constraints

- Model: `google/gemini-3.1-flash-lite` via OpenRouter, overridable with `OpenRouter.Model`.
- LLM timeout: 2.5 s (Yandex webhook limit is 3 s).
- Intents: only `add` and `unsupported`.
- One LLM call per phrase; no session history sent; no end-of-session batch.
- Empty `command` never calls the LLM.
- LLM error, timeout, or unknown intent: save the raw `command` as one item, reply "Записала как есть: ...".
- Alice reply text max 1024 characters.
- `domain.Item` and BoltDB storage stay unchanged.
- `skill_id` + `user_id` allowlist stays as in the spike.
- Tests must pass with `go test -count=1 -race ./...` (that is what `task test` runs). Do not use the real BoltDB in tests: `boltdb/bolt` crashes under `-race` with `checkptr`.

## Review Focus

1. LLM returns blank or whitespace-only items: they must not become empty list entries (adapter test, Task 1).
2. LLM returns an intent outside the enum: treated as a parse error, so the raw phrase is saved (adapter test, Task 1; handler fallback test, Task 2).
3. LLM is slow: handler must answer within the timeout and still save the raw phrase (handler timeout test, Task 2).
4. Storage write fails: user hears an error and the session stays open, no false "Записала" (handler test, Task 2).
5. Very long phrase producing many items: reply stays under 1024 characters (handler test, Task 2).

---

### Task 1: OpenRouter phrase parser

**Files:**
- Create: `internal/grocery/adapters/openrouter_phrase_parser.go`
- Create: `internal/grocery/adapters/openrouter_phrase_parser_test.go`
- Modify: `go.mod`, `go.sum` (new dependency; `go` directive becomes `1.25.10`)
- Modify: `Dockerfile:1` (`golang:1.24` -> `golang:1.25`, required by the SDK)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces:
  - `adapters.IntentAdd = "add"`, `adapters.IntentUnsupported = "unsupported"`
  - `adapters.DefaultPhraseModel = "google/gemini-3.1-flash-lite"`
  - `type adapters.ParsedPhrase struct { Intent string; Items []string; Done bool }`
  - `func adapters.NewOpenRouterPhraseParser(apiKey, model string, logger *zap.SugaredLogger, opts ...openrouter.SDKOption) *adapters.OpenRouterPhraseParser`
  - `func (*OpenRouterPhraseParser) Parse(ctx context.Context, phrase string) (adapters.ParsedPhrase, error)`

- [ ] **Step 1: Add the dependency and bump Go**

Run:
```bash
go get github.com/OpenRouterTeam/go-sdk@v0.7.25
sed -i '' 's/^FROM golang:1.24$/FROM golang:1.25/' Dockerfile
grep -n '^go ' go.mod && head -1 Dockerfile
```
Expected: `go 1.25.10` in `go.mod`, `FROM golang:1.25` in `Dockerfile`.

- [ ] **Step 2: Write the failing test**

Create `internal/grocery/adapters/openrouter_phrase_parser_test.go`. It runs the real SDK against a local fake OpenRouter server.

```go
package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"go.uber.org/zap"
)

// fakeOpenRouter answers every chat completion with content as the assistant message.
func fakeOpenRouter(t *testing.T, content string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}

		var req struct {
			Model          string          `json:"model"`
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}

		if req.Model != "test-model" || len(req.ResponseFormat) == 0 {
			t.Errorf("model = %q, response_format = %s", req.Model, req.ResponseFormat)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "gen-1",
			"object":  "chat.completion",
			"created": 1,
			"model":   "test-model",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": content},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	return srv
}

func newTestParser(srv *httptest.Server) *OpenRouterPhraseParser {
	return NewOpenRouterPhraseParser("test-key", "test-model", zap.NewNop().Sugar(), openrouter.WithServerURL(srv.URL))
}

func TestOpenRouterPhraseParser(t *testing.T) {
	srv := fakeOpenRouter(t, `{"intent":"add","items":[" Молоко, 2 л ","","  ","Хлеб"],"done":true}`)

	got, err := newTestParser(srv).Parse(context.Background(), "молоко 2 литра и хлеб и всё")
	if err != nil {
		t.Fatal(err)
	}

	if got.Intent != IntentAdd || !got.Done || !slices.Equal(got.Items, []string{"Молоко, 2 л", "Хлеб"}) {
		t.Errorf("got %+v", got)
	}
}

func TestOpenRouterPhraseParserRejectsUnknownIntent(t *testing.T) {
	srv := fakeOpenRouter(t, `{"intent":"buy","items":["Хлеб"],"done":false}`)

	if _, err := newTestParser(srv).Parse(context.Background(), "хлеб"); err == nil {
		t.Error("want error for unknown intent")
	}
}
```

If the SDK rejects the fake response because a required field is missing, add that field to the map in `fakeOpenRouter` (field list: `$(go env GOMODCACHE)/github.com/!open!router!team/go-sdk@v0.7.25/models/components/chatresult.go`). Do not change the parser to accommodate the fake.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test -count=1 ./internal/grocery/adapters/`
Expected: FAIL to compile with `undefined: OpenRouterPhraseParser`.

- [ ] **Step 4: Write the implementation**

Create `internal/grocery/adapters/openrouter_phrase_parser.go`:

```go
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	"go.uber.org/zap"
)

const (
	IntentAdd         = "add"
	IntentUnsupported = "unsupported"

	// DefaultPhraseModel is the same cheap, fast model used by swedish-news.
	DefaultPhraseModel = "google/gemini-3.1-flash-lite"

	// phraseMaxTokens bounds a runaway generation; a spoken phrase yields a few dozen tokens.
	phraseMaxTokens = 1000

	// phraseSystemPrompt is byte-identical on every call so providers with implicit
	// prompt caching can reuse it; the phrase travels in the user message.
	phraseSystemPrompt = "You interpret a Russian voice command for a shopping list. The text comes from speech " +
		"recognition: lowercase, no punctuation, numbers as digits. Set \"intent\" to \"add\" when the user names " +
		"products to buy, or only says they are finished, or the text is noise. Set it to \"unsupported\" for " +
		"anything else, such as asking what is on the list or removing items. For \"add\", split the text into " +
		"separate products. For each product write the name in nominative case with a capital letter, then, if a " +
		"quantity was said, a comma and the quantity with a short unit (л, кг, г, шт, пачка, бутылка). Keep details " +
		"that matter when buying, such as fat percentage or brand. Do not invent products or quantities. Set " +
		"\"done\" to true when the user says they are finished (for example \"всё\", \"хватит\", \"спасибо\", " +
		"\"это всё\"), including when it follows products in the same phrase. Return only products in \"items\"; " +
		"never put filler words or the finishing phrase there."
)

// ParsedPhrase is what the LLM understood from one spoken phrase.
type ParsedPhrase struct {
	Intent string   `json:"intent"`
	Items  []string `json:"items"`
	Done   bool     `json:"done"`
}

// phraseResponseFormat makes the provider enforce the JSON shape instead of relying on the prompt.
// https://openrouter.ai/docs/features/structured-outputs
var phraseResponseFormat = components.CreateResponseFormatJSONSchema(components.ChatFormatJSONSchemaConfig{
	Type: components.ChatFormatJSONSchemaConfigTypeJSONSchema,
	JSONSchema: components.ChatJSONSchemaConfig{
		Name:   "shopping_phrase",
		Strict: optionalnullable.From(openrouter.Pointer(true)),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intent": map[string]any{"type": "string", "enum": []string{IntentAdd, IntentUnsupported}},
				"items":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"done":   map[string]any{"type": "boolean"},
			},
			"required":             []string{"intent", "items", "done"},
			"additionalProperties": false,
		},
	},
})

// OpenRouterPhraseParser turns a spoken shopping phrase into list items using an LLM via OpenRouter.
type OpenRouterPhraseParser struct {
	client *openrouter.OpenRouter
	model  string
	logger *zap.SugaredLogger
}

func NewOpenRouterPhraseParser(apiKey, model string, logger *zap.SugaredLogger, opts ...openrouter.SDKOption) *OpenRouterPhraseParser {
	opts = append([]openrouter.SDKOption{openrouter.WithSecurity(apiKey)}, opts...)

	return &OpenRouterPhraseParser{client: openrouter.New(opts...), model: model, logger: logger}
}

func (p *OpenRouterPhraseParser) Parse(ctx context.Context, phrase string) (ParsedPhrase, error) {
	start := time.Now()

	res, err := p.client.Chat.Send(ctx, components.ChatRequest{
		Model: openrouter.Pointer(p.model),
		Messages: []components.ChatMessages{
			components.CreateChatMessagesSystem(components.ChatSystemMessage{
				Role:    components.ChatSystemMessageRoleSystem,
				Content: components.CreateChatSystemMessageContentStr(phraseSystemPrompt),
			}),
			components.CreateChatMessagesUser(components.ChatUserMessage{
				Role:    components.ChatUserMessageRoleUser,
				Content: components.CreateChatUserMessageContentStr(phrase),
			}),
		},
		Temperature:    optionalnullable.From(openrouter.Pointer(0.0)),
		MaxTokens:      optionalnullable.From(openrouter.Pointer(int64(phraseMaxTokens))),
		ResponseFormat: openrouter.Pointer(phraseResponseFormat),
	}, nil)
	if err != nil {
		return ParsedPhrase{}, fmt.Errorf("parse phrase: %w", err)
	}

	if res.ChatResult == nil || len(res.ChatResult.Choices) == 0 {
		return ParsedPhrase{}, fmt.Errorf("openrouter api returned no choices")
	}

	msgContent, ok := res.ChatResult.Choices[0].Message.GetContent().GetOrZero()
	if !ok || msgContent.Str == nil {
		return ParsedPhrase{}, fmt.Errorf("openrouter api returned no message content")
	}

	var out ParsedPhrase
	if err := json.Unmarshal([]byte(*msgContent.Str), &out); err != nil {
		return ParsedPhrase{}, fmt.Errorf("decode phrase response %q: %w", *msgContent.Str, err)
	}

	if out.Intent != IntentAdd && out.Intent != IntentUnsupported {
		return ParsedPhrase{}, fmt.Errorf("unknown intent %q", out.Intent)
	}

	items := out.Items[:0]

	for _, item := range out.Items {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}

	out.Items = items

	logFields := []any{"phrase", phrase, "result", out, "duration", time.Since(start)}
	if usage := res.ChatResult.Usage; usage != nil {
		logFields = append(logFields, "prompt_tokens", usage.PromptTokens, "completion_tokens", usage.CompletionTokens)
	}

	p.logger.Infow("Phrase parsed", logFields...)

	return out, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go vet ./... && go test -count=1 -race ./internal/grocery/adapters/`
Expected: `ok  	github.com/noodlensk/shopping-list/internal/grocery/adapters`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum Dockerfile internal/grocery/adapters/openrouter_phrase_parser.go internal/grocery/adapters/openrouter_phrase_parser_test.go
git commit -m "feat: add OpenRouter phrase parser for Alice"
```

---

### Task 2: Alice handler with LLM parsing

**Files:**
- Modify: `internal/grocery/ports/alice.go` (replace the echo handler and `aliceStopWords`)
- Modify: `internal/grocery/ports/alice_test.go` (replace the echo test)

**Interfaces:**
- Consumes (Task 1): `adapters.ParsedPhrase`, `adapters.IntentAdd`, `adapters.IntentUnsupported`.
- Consumes (existing): `app.Application`, `command.AddItem{Name string}`, `list.Repository`.
- Produces: `func ports.NewAliceHandler(skillID string, allowedUsers []string, parser phraseParser, application *app.Application, logger *zap.SugaredLogger) http.Handler`, where `phraseParser` is satisfied by `*adapters.OpenRouterPhraseParser`.

- [ ] **Step 1: Write the failing test**

Replace `internal/grocery/ports/alice_test.go` with:

```go
package ports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/noodlensk/shopping-list/internal/grocery/adapters"
	"github.com/noodlensk/shopping-list/internal/grocery/app"
	"github.com/noodlensk/shopping-list/internal/grocery/app/command"
	"github.com/noodlensk/shopping-list/internal/grocery/domain/list"
	"go.uber.org/zap"
)

// addOnlyRepo records added names; other repository methods are not used by the Alice handler.
type addOnlyRepo struct {
	list.Repository
	added []string
	err   error
}

func (r *addOnlyRepo) AddItem(_ context.Context, name string) error {
	if r.err != nil {
		return r.err
	}

	r.added = append(r.added, name)

	return nil
}

type fakeParser struct {
	parsed adapters.ParsedPhrase
	err    error
	block  bool
	calls  int
}

func (p *fakeParser) Parse(ctx context.Context, _ string) (adapters.ParsedPhrase, error) {
	p.calls++

	if p.block {
		<-ctx.Done()

		return adapters.ParsedPhrase{}, ctx.Err()
	}

	return p.parsed, p.err
}

func callAlice(t *testing.T, h http.Handler, skill, user, cmd string) aliceResponse {
	t.Helper()

	body := fmt.Sprintf(`{"request":{"command":%q},"session":{"skill_id":%q,"user":{"user_id":%q}},"version":"1.0"}`, cmd, skill, user)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

	var resp aliceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}

	return resp
}

func TestAliceHandler(t *testing.T) {
	manyItems := make([]string, 100)
	for i := range manyItems {
		manyItems[i] = fmt.Sprintf("Очень длинное название товара номер %d", i)
	}

	cases := []struct {
		name      string
		skill     string
		user      string
		cmd       string
		parser    fakeParser
		repoErr   error
		wantAdded []string
		wantText  string
		wantEnd   bool
		wantCalls int
	}{
		{name: "other skill", skill: "other", user: "me", cmd: "хлеб", wantText: "не для вас", wantEnd: true},
		{name: "other user", skill: "skill", user: "stranger", cmd: "хлеб", wantText: "не для вас", wantEnd: true},
		{name: "launch without phrase", skill: "skill", user: "me", cmd: "", wantText: "Что добавить?"},
		{
			name: "items", skill: "skill", user: "me", cmd: "молоко 2 литра и хлеб",
			parser:    fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd, Items: []string{"Молоко, 2 л", "Хлеб"}}},
			wantAdded: []string{"Молоко, 2 л", "Хлеб"}, wantText: "Записала: Молоко, 2 л; Хлеб. Что ещё?", wantCalls: 1,
		},
		{
			name: "items and done", skill: "skill", user: "me", cmd: "хлеб и всё",
			parser:    fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd, Items: []string{"Хлеб"}, Done: true}},
			wantAdded: []string{"Хлеб"}, wantText: "Записала: Хлеб. Готово.", wantEnd: true, wantCalls: 1,
		},
		{
			name: "done only", skill: "skill", user: "me", cmd: "все спасибо",
			parser:   fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd, Done: true}},
			wantText: "Готово.", wantEnd: true, wantCalls: 1,
		},
		{
			name: "noise", skill: "skill", user: "me", cmd: "эээ",
			parser:   fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd}},
			wantText: "Не поняла, что добавить?", wantCalls: 1,
		},
		{
			name: "unsupported", skill: "skill", user: "me", cmd: "что в списке",
			parser:   fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentUnsupported}},
			wantText: "Пока умею только добавлять. Что добавить?", wantCalls: 1,
		},
		{
			name: "parser error saves raw phrase", skill: "skill", user: "me", cmd: "молоко 2 литра",
			parser:    fakeParser{err: errors.New("boom")},
			wantAdded: []string{"молоко 2 литра"}, wantText: "Записала как есть: молоко 2 литра. Что ещё?", wantCalls: 1,
		},
		{
			name: "parser timeout saves raw phrase", skill: "skill", user: "me", cmd: "хлеб",
			parser:    fakeParser{block: true},
			wantAdded: []string{"хлеб"}, wantText: "Записала как есть: хлеб. Что ещё?", wantCalls: 1,
		},
		{
			name: "storage error", skill: "skill", user: "me", cmd: "хлеб",
			parser:   fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd, Items: []string{"Хлеб"}}},
			repoErr:  errors.New("disk full"),
			wantText: "Не получилось сохранить, попробуйте ещё раз.", wantCalls: 1,
		},
		{
			name: "many items", skill: "skill", user: "me", cmd: "много всего",
			parser:    fakeParser{parsed: adapters.ParsedPhrase{Intent: adapters.IntentAdd, Items: manyItems}},
			wantAdded: manyItems, wantText: "Записала 100 пунктов. Что ещё?", wantCalls: 1,
		},
	}

	aliceParseTimeout = 50 * time.Millisecond

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &addOnlyRepo{err: c.repoErr}
			parser := c.parser
			application := &app.Application{Commands: app.Commands{AddItem: command.NewAddItemHandler(repo)}}
			h := NewAliceHandler("skill", []string{"me"}, &parser, application, zap.NewNop().Sugar())

			start := time.Now()
			resp := callAlice(t, h, c.skill, c.user, c.cmd)

			if time.Since(start) > time.Second {
				t.Errorf("handler took %s", time.Since(start))
			}

			if !strings.Contains(resp.Response.Text, c.wantText) {
				t.Errorf("text = %q, want it to contain %q", resp.Response.Text, c.wantText)
			}

			if n := utf8.RuneCountInString(resp.Response.Text); n > aliceMaxText {
				t.Errorf("text has %d characters, limit is %d", n, aliceMaxText)
			}

			if resp.Response.EndSession != c.wantEnd {
				t.Errorf("end_session = %v, want %v", resp.Response.EndSession, c.wantEnd)
			}

			if !slices.Equal(repo.added, c.wantAdded) {
				t.Errorf("added = %q, want %q", repo.added, c.wantAdded)
			}

			if parser.calls != c.wantCalls {
				t.Errorf("parser calls = %d, want %d", parser.calls, c.wantCalls)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./internal/grocery/ports/`
Expected: FAIL to compile (`too many arguments in call to NewAliceHandler`, `undefined: aliceParseTimeout`).

- [ ] **Step 3: Write the implementation**

In `internal/grocery/ports/alice.go`, keep the `aliceRequest` and `aliceResponse` types. Delete `aliceStopWords` and the old `NewAliceHandler`. Replace the imports and everything below `aliceResponse` with:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noodlensk/shopping-list/internal/grocery/adapters"
	"github.com/noodlensk/shopping-list/internal/grocery/app"
	"github.com/noodlensk/shopping-list/internal/grocery/app/command"
	"go.uber.org/zap"
)
```

```go
// aliceParseTimeout keeps the LLM call inside the 3 s Yandex gives the webhook to answer.
var aliceParseTimeout = 2500 * time.Millisecond

// aliceMaxText is the Yandex limit for response.text.
const aliceMaxText = 1024

type phraseParser interface {
	Parse(ctx context.Context, phrase string) (adapters.ParsedPhrase, error)
}

// NewAliceHandler serves the webhook of a private Yandex Dialogs skill.
// Each phrase goes to the LLM; the items it returns are added to the shopping list.
func NewAliceHandler(skillID string, allowedUsers []string, parser phraseParser, application *app.Application, logger *zap.SugaredLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var req aliceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		resp := aliceResponse{Version: req.Version}

		// Yandex does not sign requests; skill_id + user_id allowlist is the only gate.
		if req.Session.SkillID != skillID || !slices.Contains(allowedUsers, req.Session.User.UserID) {
			logger.Warnw("Alice: not allowed", "skill_id", req.Session.SkillID, "user_id", req.Session.User.UserID)
			resp.Response.Text, resp.Response.EndSession = "Извините, этот список не для вас.", true
		} else {
			resp.Response.Text, resp.Response.EndSession = aliceReply(r.Context(), req.Request.Command, parser, application, logger)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		logger.Infow("Alice: handled", "command", req.Request.Command, "reply", resp.Response.Text, "duration", time.Since(start))
	})
}

// aliceReply returns the text Alice says and whether the session ends.
func aliceReply(ctx context.Context, cmd string, parser phraseParser, application *app.Application, logger *zap.SugaredLogger) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "Что добавить?", false
	}

	parseCtx, cancel := context.WithTimeout(ctx, aliceParseTimeout)
	defer cancel()

	prefix := "Записала"

	parsed, err := parser.Parse(parseCtx, cmd)
	if err != nil {
		logger.Warnw("Alice: parse failed, saving phrase as is", "command", cmd, "error", err)

		parsed, prefix = adapters.ParsedPhrase{Intent: adapters.IntentAdd, Items: []string{cmd}}, "Записала как есть"
	}

	if parsed.Intent == adapters.IntentUnsupported {
		if parsed.Done {
			return "Пока умею только добавлять. Готово.", true
		}

		return "Пока умею только добавлять. Что добавить?", false
	}

	for _, item := range parsed.Items {
		if err := application.Commands.AddItem.Handle(ctx, command.AddItem{Name: item}); err != nil {
			logger.Errorw("Alice: add item", "item", item, "error", err)

			return "Не получилось сохранить, попробуйте ещё раз.", false
		}
	}

	if len(parsed.Items) == 0 {
		if parsed.Done {
			return "Готово.", true
		}

		return "Не поняла, что добавить?", false
	}

	text := prefix + ": " + strings.Join(parsed.Items, "; ") + "."
	if utf8.RuneCountInString(text) > aliceMaxText-20 { // room for the suffix below
		text = fmt.Sprintf("%s %d пунктов.", prefix, len(parsed.Items))
	}

	if parsed.Done {
		return text + " Готово.", true
	}

	return text + " Что ещё?", false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go vet ./internal/grocery/... && go test -count=1 -race ./internal/grocery/...`
Expected: both packages `ok`. `go vet ./...` on the whole module still fails until Task 3 updates `main.go`; that is expected here.

- [ ] **Step 5: Commit**

```bash
git add internal/grocery/ports/alice.go internal/grocery/ports/alice_test.go
git commit -m "feat: parse Alice phrases with LLM and add items"
```

---

### Task 3: Config and wiring

**Files:**
- Modify: `internal/common/config/config.go` (new `OpenRouter` section and validation)
- Modify: `main.go:57-64` (build the parser, pass it to the handler)
- Modify: `config.example.yaml` (fix the missing newline after `Path: ./grocery.db`, add `OpenRouter`)

**Interfaces:**
- Consumes (Task 1): `adapters.NewOpenRouterPhraseParser`, `adapters.DefaultPhraseModel`.
- Consumes (Task 2): `ports.NewAliceHandler(skillID, allowedUsers, parser, application, logger)`.
- Produces: config keys `OpenRouter.APIKey`, `OpenRouter.Model`.

- [ ] **Step 1: Add the config section**

In `internal/common/config/config.go`, add to `Config` after the `Alice` struct:

```go
	// OpenRouter is required when Alice is enabled: it parses spoken phrases into items.
	OpenRouter struct {
		APIKey string
		Model  string
	}
```

Add to the validation block, next to the existing `Alice` check:

```go
	if cfg.Alice.Listen != "" && cfg.OpenRouter.APIKey == "" {
		validationErrs = append(validationErrs, "OpenRouter.APIKey is required when Alice.Listen is set")
	}
```

- [ ] **Step 2: Wire the parser in `main.go`**

Add `"cmp"` and `"github.com/noodlensk/shopping-list/internal/grocery/adapters"` to the imports. Replace the handler construction inside `if cfg.Alice.Listen != "" {`:

```go
		parser := adapters.NewOpenRouterPhraseParser(
			cfg.OpenRouter.APIKey,
			cmp.Or(cfg.OpenRouter.Model, adapters.DefaultPhraseModel),
			logger.With("component", "alice-parser"),
		)
		handler := ports.NewAliceHandler(cfg.Alice.SkillID, cfg.Alice.AllowedUsers, parser, app, logger)
```

- [ ] **Step 3: Fix and extend `config.example.yaml`**

The file currently has `Path: ./grocery.db# Optional: webhook ...` on one line, which makes the example storage path wrong. Rewrite the tail of the file so it reads:

```yaml
Storage:
  Path: ./grocery.db
# Optional: webhook for a private Yandex Alice skill (Yandex Dialogs).
# Remove the section to disable. Must be reachable from Yandex over HTTPS (put a reverse proxy in front).
Alice:
  Listen: ":8080"
  SkillID: "00000000-0000-0000-0000-000000000000"
  AllowedUsers: # session.user.user_id, rejected ids are logged
  - "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
# Required when Alice is enabled.
OpenRouter:
  APIKey: "sk-or-v1-..."
  Model: "google/gemini-3.1-flash-lite"
```

- [ ] **Step 4: Verify the whole module**

Run: `gofmt -l . && go vet ./... && go test -count=1 -race ./... && go build -o /dev/null .`
Expected: no `gofmt` output, `go vet` clean, all packages `ok` or `[no test files]`, build succeeds.

Then check the example config parses and the new validation fires. A config without `OpenRouter` but with `Alice.Listen` must fail:

```bash
grep -v -E '^(OpenRouter:|  APIKey:|  Model:|# Required when Alice)' config.example.yaml > /tmp/no-openrouter.yaml
go run . -config /tmp/no-openrouter.yaml 2>&1 | grep -o 'OpenRouter.APIKey is required when Alice.Listen is set'
```
Expected: the grep prints the message.

- [ ] **Step 5: Commit**

```bash
git add internal/common/config/config.go main.go config.example.yaml
git commit -m "feat: configure OpenRouter for Alice phrase parsing"
```

---

### Task 4: Deploy and check on the Station

Manual steps. Pushing `master` triggers CI (git-cliff version bump, image build, homelab tag update). Ask the user before each push.

- [ ] **Step 1: User creates the OpenRouter key**

The user creates a dedicated key at https://openrouter.ai/settings/keys for shopping-list.

- [ ] **Step 2: Add the key to homelab**

In `../homelab/cloud/argo/apps/shopping-list.yaml`, under `config:` after the `Alice:` block, add:

```yaml
          OpenRouter:
            APIKey: "<key from step 1>"
```

The user commits and pushes homelab (auto mode blocks this for the agent).

- [ ] **Step 3: Push shopping-list**

```bash
git push origin master
```

Wait until the homelab tag for shopping-list is bumped and the deployment runs the new image:

```bash
kubectl -n shopping-list get deploy shopping-list -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

- [ ] **Step 4: Restart after config sync**

The chart does not roll pods on ConfigMap changes. Once `kubectl -n shopping-list get cm shopping-list-config -o yaml | grep -c APIKey` prints `1`, the user runs:

```bash
kubectl -n shopping-list rollout restart deploy/shopping-list && kubectl -n shopping-list rollout status deploy/shopping-list
```

- [ ] **Step 5: Check with a real request**

```bash
curl -s -X POST https://shopping-list.noodlensk.dev/ -d '{"request":{"command":"молоко 2 литра пачку масла 82 процента полкило фарша и хлеба и 3 яйца"},"session":{"skill_id":"17a778ab-0430-49c4-9b98-a28c7b278a41","user":{"user_id":"A3E1F715B03CC7917880DAD2FB175B5B0B508CBF3266DA73186D77B5FCCC31CE"}},"version":"1.0"}'
kubectl -n shopping-list logs deploy/shopping-list --tail=5 | grep -E "Phrase parsed|Alice: handled"
```

Expected: a reply listing five items, `Phrase parsed` with `duration` well under 2.5 s. Repeat with `все спасибо` (expect "Готово.", `end_session: true`) and `что в списке` (expect "Пока умею только добавлять"). The curl calls add real items; remove them in Telegram afterwards.

- [ ] **Step 6: User checks on the Station**

The user says the same phrases to the Station, checks that items appear in Telegram, and listens to how TTS reads `2 л` and `1 пачка`. If it sounds wrong, that is a follow-up (spec: fill `tts` with spelled-out units).
