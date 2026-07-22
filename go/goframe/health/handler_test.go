package goframehealth_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	goframehealth "github.com/yueli-official/foundation/go/goframe/health"
	"github.com/yueli-official/foundation/go/health"
	"github.com/yueli-official/foundation/go/problem"
)

func TestHandlerServesRawReadyAndProblemNotReady(t *testing.T) {
	descriptor := problem.MustDescriptor(
		problem.MustKind("common.not_ready", http.StatusServiceUnavailable),
		"https://errors.example.test/problems/common.not_ready",
	)
	for _, test := range []struct {
		name   string
		check  health.Check
		status int
	}{
		{"ready", func(context.Context) error { return nil }, 200},
		{"not-ready", func(context.Context) error { return errors.New("private db detail") }, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := health.MustRunner(map[string]health.Check{"database": test.check}, health.RunnerOptions{Timeout: time.Second})
			handler, err := goframehealth.Handler(goframehealth.HandlerOptions{Runner: runner, NotReady: descriptor, TraceID: func(*ghttp.Request) string { return "trace-health" }})
			if err != nil {
				t.Fatal(err)
			}
			server := g.Server("foundation-health-" + test.name)
			server.SetAddr("127.0.0.1:0")
			server.SetDumpRouterMap(false)
			server.Group("/", func(group *ghttp.RouterGroup) { group.GET("/readyz", handler) })
			server.Start()
			defer server.Shutdown()
			response, err := g.Client().Get(context.Background(), fmt.Sprintf("http://127.0.0.1:%d/readyz", server.GetListenedPort()))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Close()
			body := response.ReadAll()
			if response.StatusCode != test.status {
				t.Fatalf("status/body = %d/%s", response.StatusCode, body)
			}
			if test.status == 503 {
				value, err := problem.Decode(body)
				if err != nil || value.Code != "common.not_ready" {
					t.Fatalf("Problem = %#v, %v", value, err)
				}
			} else if string(body) != `{"status":"ready"}` {
				t.Fatalf("ready body = %s", body)
			}
		})
	}
}
