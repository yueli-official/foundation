// Package health runs dependency probes without depending on an HTTP framework
// or a concrete database/Redis client.
package health

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"time"
)

type Check func(context.Context) error

type PanicObserver func(name string, recovered any)

type RunnerOptions struct {
	Timeout time.Duration
	OnPanic PanicObserver
}

type probe struct {
	name     string
	check    Check
	inFlight atomic.Bool
}

// Runner snapshots its checks and keeps at most one invocation of each check
// in flight, even when a broken dependency ignores context cancellation.
type Runner struct {
	probes  []*probe
	timeout time.Duration
	onPanic PanicObserver
}

type Report struct {
	Failed []string `json:"failed,omitempty"`
}

func (report Report) Ready() bool { return len(report.Failed) == 0 }

func NewRunner(checks map[string]Check, options RunnerOptions) (*Runner, error) {
	if options.Timeout <= 0 {
		return nil, errors.New("health: Timeout must be positive")
	}
	names := make([]string, 0, len(checks))
	for name := range checks {
		if name == "" {
			return nil, errors.New("health: check name is empty")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	probes := make([]*probe, 0, len(names))
	for _, name := range names {
		probes = append(probes, &probe{name: name, check: checks[name]})
	}
	return &Runner{probes: probes, timeout: options.Timeout, onPanic: options.OnPanic}, nil
}

func MustRunner(checks map[string]Check, options RunnerOptions) *Runner {
	runner, err := NewRunner(checks, options)
	if err != nil {
		panic(err)
	}
	return runner
}

func (runner *Runner) Run(parent context.Context) Report {
	if runner == nil || len(runner.probes) == 0 {
		return Report{Failed: []string{"configuration"}}
	}
	ctx, cancel := context.WithTimeout(parent, runner.timeout)
	defer cancel()
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(runner.probes))
	pending := make(map[string]struct{}, len(runner.probes))
	failed := make([]string, 0)
	for _, item := range runner.probes {
		pending[item.name] = struct{}{}
		if !item.inFlight.CompareAndSwap(false, true) {
			results <- result{name: item.name, err: errors.New("health: check already in flight")}
			continue
		}
		go func(item *probe) {
			var err error
			defer func() {
				if recovered := recover(); recovered != nil {
					if runner.onPanic != nil {
						runner.onPanic(item.name, recovered)
					}
					err = errors.New("health: check panicked")
				}
				item.inFlight.Store(false)
				results <- result{name: item.name, err: err}
			}()
			if item.check == nil {
				err = errors.New("health: check is nil")
				return
			}
			err = item.check(ctx)
		}(item)
	}
	for len(pending) != 0 {
		select {
		case item := <-results:
			if _, exists := pending[item.name]; !exists {
				continue
			}
			delete(pending, item.name)
			if item.err != nil {
				failed = append(failed, item.name)
			}
		case <-ctx.Done():
			for name := range pending {
				failed = append(failed, name)
			}
			pending = nil
		}
	}
	sort.Strings(failed)
	return Report{Failed: failed}
}
