package mq

import (
	"context"
	"errors"
	"testing"

	"campus-service-platform/internal/service"
)

type flakyProc struct {
	fails int
	calls int
}

func (f *flakyProc) ProcessSeckillOrder(ctx context.Context, msg service.SeckillOrderMsg) error {
	f.calls++
	if f.calls <= f.fails {
		return errors.New("db down")
	}
	return nil
}

// 重试 3 次（指数退避）后成功：共 4 次调用。
func TestProcessWithRetryRecovers(t *testing.T) {
	proc := &flakyProc{fails: 2}
	c := &Consumer{proc: proc}
	if err := c.processWithRetry(context.Background(), service.SeckillOrderMsg{Id: 1}); err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if proc.calls != 3 {
		t.Fatalf("调用次数 = %d, want 3", proc.calls)
	}
}

// 3 次重试仍失败 → 返回错误（上层投 DLT）。
func TestProcessWithRetryGivesUp(t *testing.T) {
	proc := &flakyProc{fails: 99}
	c := &Consumer{proc: proc}
	if err := c.processWithRetry(context.Background(), service.SeckillOrderMsg{Id: 2}); err == nil {
		t.Fatal("应返回错误")
	}
	if proc.calls != 4 {
		t.Fatalf("调用次数 = %d, want 4（1+3 重试）", proc.calls)
	}
}
