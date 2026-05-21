package user

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAlertKeywords(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "[]"},
		{"a,b", `["a","b"]`},
		{`["x"]`, `["x"]`},
		{"a, b , c", `["a","b","c"]`},
	}
	for _, tc := range tests {
		got := normalizeAlertKeywords(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeAlertKeywords(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if !json.Valid([]byte(got)) {
			t.Fatalf("invalid json: %q", got)
		}
	}
}

func TestBuildNotifyConf(t *testing.T) {
	conf, err := buildNotifyConf(alertRuleReq{NotifyType: "email", NotifyEmail: "a@b.com"})
	if err != nil || conf != `{"email":"a@b.com"}` {
		t.Fatalf("email conf = %q err=%v", conf, err)
	}
	_, err = buildNotifyConf(alertRuleReq{NotifyType: "email"})
	if err == nil {
		t.Fatal("expected email required error")
	}
}

func TestNormalizeSentiment(t *testing.T) {
	if normalizeSentiment("all") != "" || normalizeSentiment("") != "" {
		t.Fatal("all/empty should normalize to empty")
	}
	if normalizeSentiment("negative") != "negative" {
		t.Fatal("negative should pass through")
	}
}

func TestFormatNotifyConf(t *testing.T) {
	got := formatNotifyConf("email", `{"email":"x@y.com"}`)
	if got != "x@y.com" {
		t.Fatalf("got %q", got)
	}
}
