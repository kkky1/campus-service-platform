package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

func TestJobStoreEvictsOldJobs(t *testing.T) {
	s := newJobStore()
	for i := 0; i < maxJobs+5; i++ {
		s.add("git-pull")
	}
	list := s.list()
	if len(list) != maxJobs {
		t.Fatalf("保留任务数 = %d, want %d", len(list), maxJobs)
	}
	if list[0].ID != fmt.Sprintf("%d", maxJobs+5) {
		t.Fatalf("最新任务应排第一: %s", list[0].ID)
	}
	if list[len(list)-1].ID != "6" {
		t.Fatalf("最旧任务应被淘汰: %s", list[len(list)-1].ID)
	}
}

func TestJobAppendCapsLines(t *testing.T) {
	j := &Job{ID: "1", Status: "running", Lines: []string{}}
	for i := 0; i < maxJobLines+100; i++ {
		j.append(fmt.Sprintf("line-%d", i))
	}
	if len(j.Lines) > maxJobLines {
		t.Fatalf("日志行数 = %d, want <= %d", len(j.Lines), maxJobLines)
	}
	if last := j.Lines[len(j.Lines)-1]; last != fmt.Sprintf("line-%d", maxJobLines+99) {
		t.Fatalf("最新日志应保留: %s", last)
	}
}

// 并发追加日志 + /api/jobs 编码，配合 -race 回归 data race。
func TestJobConcurrentMarshal(t *testing.T) {
	j := &Job{ID: "1", Status: "running", Lines: []string{}}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			j.append("line")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if _, err := json.Marshal(j); err != nil {
				t.Errorf("marshal: %v", err)
			}
		}
	}()
	wg.Wait()
}
