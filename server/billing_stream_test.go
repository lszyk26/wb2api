package main

// billing_test.go — 计费与计数回归测试（对应 issue #1 / #2）
//
// 覆盖：上游没有 usage 时，按内容长度兜底估算 token（不能按 1 token 计费）

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 上游只回内容、完全不回 usage 的流式响应
const streamNoUsage = "data: {\"choices\":[{\"delta\":{\"content\":\"这是一个用来验证兜底计费的偏长回答，需要足够多的字符\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"才能让估算出来的 token 数明显大于 1，否则测试没意义。\"}}]}\n\n" +
	"data: [DONE]\n\n"

func mkKey(t *testing.T, u *User, hash string) *APIKey {
	t.Helper()
	k := &APIKey{UserID: u.ID, Name: "test-key", Prefix: "sk-wb-", Hash: hash}
	if err := db.Create(k).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}
	return k
}

func TestProxyStreamChargesWithoutUsage(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "streamuser", 10000, 0)
	k := mkKey(t, u, "hash-stream")

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamNoUsage)),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")

	proxyStream(rec, resp, u, k, "glm-5.3-flash", time.Now(), req)

	var row LogRow
	if err := db.Order("id desc").First(&row).Error; err != nil {
		t.Fatalf("没有写出日志行: %v", err)
	}
	if row.PromptTok <= 1 || row.ComplTok <= 1 {
		t.Fatalf("兜底估算失效：pt=%d ct=%d（应为按内容长度估算出的较大值）", row.PromptTok, row.ComplTok)
	}
	if row.Points <= 0 {
		t.Fatalf("没有 usage 的流式请求扣点必须 > 0，实际 %v", row.Points)
	}
	// 客户端仍然要拿到完整的流
	if body := rec.Body.String(); !strings.Contains(body, "[DONE]") {
		t.Fatalf("透传给客户端的流不完整: %q", body)
	}
}
