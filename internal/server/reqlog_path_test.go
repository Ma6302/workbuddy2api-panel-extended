package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/reqlog"
)

// 三种入站协议共用同一条 chat 管线与统计，归档事件必须记**真实**路径：
// 既不能把 /v1/messages、/v1/responses 的流量丢掉（面板运行日志整片消失），
// 也不能把它们统一写成 /v1/chat/completions（来源无法区分）。
func TestRequestMetricsRecordsAllInboundPaths(t *testing.T) {
	cases := []struct {
		path string
		body string
	}{
		{"/v1/chat/completions", `{"model":"glm-5.2","stream":true,"messages":[{"role":"user","content":"hi"}]}`},
		{"/v1/messages", `{"model":"glm-5.2","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`},
		{"/messages", `{"model":"glm-5.2","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`},
		{"/v1/responses", `{"model":"glm-5.2","input":"hi","stream":true}`},
	}
	for _, tc := range cases {
		up := newFakeUpstream(t, func(string) (int, string, bool) { return 200, sseOK, true })
		reqLog := reqlog.New(reqlog.Config{})
		h := NewHandler(Config{
			Pool:       testPoolWith(&auth.Auth{UID: "u1", AccessToken: "at1", ExpiresAt: 9999999999}),
			Upstream:   up,
			RequestLog: reqLog,
		})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: code=%d body=%s", tc.path, rec.Code, rec.Body)
		}
		s := reqLog.Snapshot()
		if len(s.Recent) != 1 {
			t.Fatalf("%s: recent=%d, want 1（该路径未进归档）", tc.path, len(s.Recent))
		}
		if got := s.Recent[0].Path; got != tc.path {
			t.Errorf("%s: event path=%q, want %q", tc.path, got, tc.path)
		}
		if got := s.Recent[0].Model; got != "glm-5.2" {
			t.Errorf("%s: event model=%q, want glm-5.2", tc.path, got)
		}
	}
}
