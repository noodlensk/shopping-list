# Alice skill: LLM item parsing

Date: 2026-10-01
Status: draft, awaiting review

## Goal

Say a shopping phrase to a Yandex Station at home ("Алиса, попроси <skill> молоко два литра и хлеб") and have
clean items appear in the Telegram shopping list, ready to tick off in the store.

Phase 1 (spike, done) proved the private skill reaches the bot: requests from the Station arrive at
`https://shopping-list.noodlensk.dev/`, the bot answers in under 1 ms, and the user perceives about 2 s
from end of speech to Alice's reply.

This document covers phase 2: turning the spoken phrase into list items. Phase 3 ("что в списке" by
voice, removing items) is out of scope.

## What the spike showed

- Yandex sends no punctuation. A real phrase arrived as
  `молоко 2 литра пачку масла 82 процента полкило фарша и хлеба и 3 яйца`. Rule-based splitting cannot
  tell where one item ends and the next begins.
- `request.command` is lowercased, numbers are already digits; `nlu.entities` has `YANDEX.NUMBER` only.
  No lemmatization ("фарша", "хлеба").
- People end the conversation in free form ("все спасибо"), which exact stop-word matching missed.
- Yandex gives the webhook 3 s to answer and has no streaming. The backend cannot speak to the user
  without a new request from the user.

## Decisions

| Topic | Decision |
|---|---|
| Item format | Plain text in normalized form: `Молоко, 2 л`, `Масло 82%, 1 пачка`, `Хлеб`. `domain.Item` is unchanged. No merging of duplicates. |
| Who interprets the phrase | The LLM decides everything: intent, item split, normalization, and whether the user is done. No hand-written parsing or stop words. |
| Intents | Only what phase 2 handles: `add` and `unsupported`. Phase 3 extends the enum (`list`, `complete`); the schema lives in code and prompt only, so changing it later is cheap. |
| When the LLM is called | Once per phrase (each phrase is its own webhook request). No end-of-session batch: a session can end by silence without a final request, and per-phrase replies confirm items immediately. Previous phrases of the session are not sent. |
| LLM | `google/gemini-3.1-flash-lite` via OpenRouter, same Go SDK and structured-output approach as `../swedish-news`. Separate API key for this project. |
| Sync vs async | Synchronous: wait for the LLM, then answer with what was added. |
| Timeout | 2.5 s on the LLM call, to stay inside Yandex's 3 s webhook limit. |
| Users | One Yandex account (`AllowedUsers` with one id), used from all speakers at home. |

## Flow

1. Webhook receives a request; `skill_id` + `user_id` allowlist check stays as in the spike.
2. Empty `command` (skill launched without a phrase): answer "Что добавить?", keep the session open.
   No LLM call.
3. Otherwise call the LLM with `command` and a 2.5 s context timeout. It returns:

   ```json
   {"intent": "add", "items": ["Молоко, 2 л", "Хлеб"], "done": false}
   ```

4. `intent: unsupported` (for example "что в списке", "удали молоко"): add nothing, answer
   "Пока умею только добавлять. Что добавить?", session stays open (ends if `done`).
5. `intent: add`: add each item with `Commands.AddItem`, then answer:
   - items, not done: "Записала: молоко, 2 л; хлеб. Что ещё?", session stays open;
   - items, done: "Записала: хлеб. Готово.", session ends;
   - no items, done: "Готово.", session ends;
   - no items, not done (noise, "эээ"): "Не поняла, что добавить?", session stays open.
   If the reply would exceed 1024 characters, say "Записала N пунктов" instead of listing them.
6. LLM error, timeout, or an unknown intent value: add the raw `command` as one item and answer "Записала как есть: <command>.
   Что ещё?". Nothing is lost; a wrong item is one tap to remove in Telegram. Known edge: during an
   OpenRouter outage "все спасибо" also becomes an item.

## Components

- `internal/grocery/adapters/openrouter_item_parser.go`: `Parse(ctx, phrase string) (Parsed, error)`
  where `Parsed` is `{Intent string; Items []string; Done bool}`. Uses `openrouter.Chat.Send` with a
  strict JSON schema (`intent: "add" | "unsupported"`, `items: string[]`, `done: boolean`, all
  required, no extra properties). The system prompt is a
  constant so provider-side prompt caching can reuse it. Logs latency and token usage per call.
- `internal/grocery/ports/alice.go`: replaces the echo handler with the flow above. Takes the parser
  through a one-method interface declared in `ports`, so the handler test can use a fake.
- `internal/common/config/config.go`: new `OpenRouter` section with `APIKey` and `Model` (default
  `google/gemini-3.1-flash-lite`). `APIKey` is required when `Alice.Listen` is set.
- `main.go`: builds the parser and passes it to the Alice handler.
- `homelab/cloud/argo/apps/shopping-list.yaml`: add `OpenRouter.APIKey` (user creates the key) and
  restart the pod after Argo syncs, since the chart does not roll pods on config changes.

## System prompt (draft)

> You interpret a Russian voice command for a shopping list. The text comes from speech
> recognition: lowercase, no punctuation, numbers as digits. Set "intent" to "add" when the user names
> products to buy, or only says they are finished, or the text is noise. Set it to "unsupported" for
> anything else, such as asking what is on the list or removing items. For "add", split the text into
> separate products. For each
> product write the name in nominative case with a capital letter, then, if a quantity was said, a
> comma and the quantity with a short unit (л, кг, г, шт, пачка, бутылка). Keep details that matter
> when buying, such as fat percentage or brand. Do not invent products or quantities. Set "done" to
> true when the user says they are finished (for example "всё", "хватит", "спасибо", "это всё"),
> including when it follows products in the same phrase. Return only products in "items"; never put
> filler words or the finishing phrase there.

## Testing

- Handler test with a fake parser: items reach `AddItem`; parser error adds the raw phrase;
  `unsupported` adds nothing and keeps the session; `done` ends the session; empty command does not call the parser; allowlist rejects other skills and users.
- `go test -race ./...` passes.
- Manual check against real OpenRouter with the spike phrases, including
  `молоко 2 литра пачку масла 82 процента полкило фарша и хлеба и 3 яйца`, `все спасибо` and
  `что в списке`; latency
  read from logs to confirm the 2.5 s timeout has headroom.
- Listen on the Station how TTS reads short units ("2 л", "1 пачка"). If it sounds wrong, fill the
  response `tts` field with spelled-out units; not planned up front.

## Out of scope

- Reading the list by voice, removing items (phase 3).
- Telegram notification when items are added by voice.
- Merging duplicates or summing quantities.
- Chart fixes in homelab (`checksum/config` annotation, readiness probe).
