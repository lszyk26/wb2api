package main

// 覆盖：logRow 的 calls 必须由数据库自增，不能用请求开始时的内存快照 +1

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogRowCallsNotLost(t *testing.T) {
	setupTestDB(t)
	u := newUser(t, "keyuser", 0, 0)
	k := mkKey(t, u, "hash-calls")

	// 模拟两个并发请求：它们各自读到的是同一个 Calls=0 的快照
	stale := *k
	req := httptest.NewRequest("GET", "/v1/models", nil)
	logRow(u, &stale, "m", 0, 0, 0, 200, "", time.Now(), 0, false, req)
	logRow(u, &stale, "m", 0, 0, 0, 200, "", time.Now(), 0, false, req)

	var got APIKey
	if err := db.First(&got, k.ID).Error; err != nil {
		t.Fatalf("读回 key 失败: %v", err)
	}
	if got.Calls != 2 {
		t.Fatalf("calls 应为 2（数据库自增），实际 %d —— 说明仍在用内存快照回写", got.Calls)
	}
}
