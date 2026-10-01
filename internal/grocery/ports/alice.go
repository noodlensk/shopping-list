package ports

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"time"

	"go.uber.org/zap"
)

// aliceRequest is the subset of the Yandex Dialogs webhook request we use.
// https://yandex.ru/dev/dialogs/alice/doc/ru/request
type aliceRequest struct {
	Request struct {
		Command string `json:"command"`
	} `json:"request"`
	Session struct {
		SkillID string `json:"skill_id"`
		User    struct {
			UserID string `json:"user_id"`
		} `json:"user"`
	} `json:"session"`
	Version string `json:"version"`
}

type aliceResponse struct {
	Response struct {
		Text       string `json:"text"`
		EndSession bool   `json:"end_session"`
	} `json:"response"`
	Version string `json:"version"`
}

// aliceStopWords end the session; any other phrase keeps it open for the next item.
var aliceStopWords = []string{"всё", "все", "хватит", "стоп", "спасибо", "это всё", "это все", "нет", "ничего"}

// NewAliceHandler serves the webhook of a private Yandex Dialogs skill.
// ponytail: spike, echoes the phrase and logs the raw request to learn the real payload and latency.
// Replace with parsing + AddItem once the skill is proven to work.
func NewAliceHandler(skillID string, allowedUsers []string, logger *zap.SugaredLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		var req aliceRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		resp := aliceResponse{Version: req.Version}
		resp.Response.EndSession = true

		// Yandex does not sign requests; skill_id + user_id allowlist is the only gate.
		if req.Session.SkillID != skillID || !slices.Contains(allowedUsers, req.Session.User.UserID) {
			logger.Warnw("Alice: not allowed", "skill_id", req.Session.SkillID, "user_id", req.Session.User.UserID)
			resp.Response.Text = "Извините, этот список не для вас."
		} else {
			logger.Infow("Alice: request", "raw", json.RawMessage(raw))

			switch cmd := req.Request.Command; {
			case cmd == "":
				resp.Response.Text, resp.Response.EndSession = "Что добавить?", false
			case slices.Contains(aliceStopWords, cmd):
				resp.Response.Text = "Готово."
			default:
				resp.Response.Text, resp.Response.EndSession = "Слышу: "+cmd+". Что ещё?", false
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		logger.Infow("Alice: handled", "duration", time.Since(start))
	})
}
