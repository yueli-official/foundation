package health

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerIsStableAndContainsPanics(t *testing.T) {
	var panics atomic.Int32
	runner := MustRunner(map[string]Check{
		"redis":    func(context.Context) error { return errors.New("private detail") },
		"database": func(context.Context) error { return nil },
		"asset":    func(context.Context) error { panic("driver invariant") },
	}, RunnerOptions{Timeout: time.Second, OnPanic: func(string, any) { panics.Add(1) }})
	report := runner.Run(context.Background())
	if !reflect.DeepEqual(report.Failed, []string{"asset", "redis"}) || panics.Load() != 1 {
		t.Fatalf("report/panics = %#v/%d", report, panics.Load())
	}
}

func TestRunnerReturnsAtDeadlineAndBoundsNonCooperativeChecks(t *testing.T) {
	blocked := make(chan struct{})
	var starts atomic.Int32
	runner := MustRunner(map[string]Check{"database": func(context.Context) error {
		starts.Add(1)
		<-blocked
		return nil
	}}, RunnerOptions{Timeout: 10 * time.Millisecond})
	started := time.Now()
	if report := runner.Run(context.Background()); !reflect.DeepEqual(report.Failed, []string{"database"}) {
		t.Fatalf("first report = %#v", report)
	}
	if report := runner.Run(context.Background()); !reflect.DeepEqual(report.Failed, []string{"database"}) {
		t.Fatalf("second report = %#v", report)
	}
	if time.Since(started) > 250*time.Millisecond || starts.Load() != 1 {
		t.Fatalf("elapsed/starts = %s/%d", time.Since(started), starts.Load())
	}
	close(blocked)
}

func TestRunnerFailsClosedWithoutChecks(t *testing.T) {
	runner := MustRunner(nil, RunnerOptions{Timeout: time.Second})
	if report := runner.Run(context.Background()); !reflect.DeepEqual(report.Failed, []string{"configuration"}) {
		t.Fatalf("report = %#v", report)
	}
}
