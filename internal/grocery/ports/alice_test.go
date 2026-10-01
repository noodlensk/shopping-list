package ports

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestAliceHandler(t *testing.T) {
	h := NewAliceHandler("skill", []string{"me"}, zap.NewNop().Sugar())

	call := func(skill, user, cmd string) string {
		body := fmt.Sprintf(`{"request":{"command":%q},"session":{"skill_id":%q,"user":{"user_id":%q}},"version":"1.0"}`, cmd, skill, user)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

		return rec.Body.String()
	}

	cases := []struct{ skill, user, want string }{
		{"skill", "me", "Слышу: молоко"},
		{"skill", "stranger", "не для вас"},
		{"other", "me", "не для вас"},
	}

	for _, c := range cases {
		if got := call(c.skill, c.user, "молоко"); !strings.Contains(got, c.want) {
			t.Errorf("skill=%q user=%q: got %s, want %q", c.skill, c.user, got, c.want)
		}
	}
}
