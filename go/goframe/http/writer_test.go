package goframehttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	goframehttp "github.com/yueli-official/foundation/go/goframe/http"
	"github.com/yueli-official/foundation/go/problem"
)

func TestWriterServesProblemContractFromRealGoFrameServer(t *testing.T) {
	kind := problem.MustKind("blog.slug_taken", 409)
	value, err := problem.New(
		kind,
		"https://docs.example.test/problems/blog.slug_taken",
		"trace-goframe-1",
		problem.Parameters{"slug": "hello"},
		problem.Violation{
			Pointer: "/slug",
			Code:    "validation.unique",
			Params:  problem.Parameters{"value": "hello"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	writer := goframehttp.MustWriter(goframehttp.WriterOptions{})

	server := g.Server("foundation-problem-writer")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/problem", func(request *ghttp.Request) {
			if err := writer.Write(request, value); err != nil {
				t.Errorf("Write() error = %v", err)
			}
		})
	})
	server.Start()
	defer server.Shutdown()

	response, err := g.Client().Get(
		context.Background(),
		fmt.Sprintf("http://127.0.0.1:%d/problem", server.GetListenedPort()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	body := response.ReadAll()

	if response.StatusCode != 409 {
		t.Fatalf("status = %d, body = %s", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header.Get("X-Trace-Id"); got != value.TraceID {
		t.Fatalf("X-Trace-Id = %q", got)
	}
	decoded, err := problem.Decode(body)
	if err != nil {
		t.Fatalf("response Decode() error = %v, body = %s", err, body)
	}
	if decoded.Status != response.StatusCode || decoded.Code != kind.Code() {
		t.Fatalf("decoded = %#v", decoded)
	}
	if strings.Contains(string(body), "internal error") {
		t.Fatalf("diagnostic leaked in body: %s", body)
	}
}

func TestWriterRejectsInvalidConfigurationAndOversizedBodies(t *testing.T) {
	if _, err := goframehttp.NewWriter(goframehttp.WriterOptions{MaxBodyBytes: -1}); err == nil {
		t.Fatal("NewWriter() accepted a negative body limit")
	}
	if _, err := goframehttp.NewWriter(goframehttp.WriterOptions{TraceHeader: "Bad\nHeader"}); err == nil {
		t.Fatal("NewWriter() accepted an invalid trace header")
	}

	kind := problem.MustKind("common.internal", 500)
	value, err := problem.New(kind, "https://example.test/problems/common.internal", "trace-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	writer := goframehttp.MustWriter(goframehttp.WriterOptions{MaxBodyBytes: 8})

	server := g.Server("foundation-problem-writer-limit")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/problem", func(request *ghttp.Request) {
			if err := writer.Write(request, value); !errors.Is(err, goframehttp.ErrBodyTooLarge) {
				t.Errorf("Write() error = %v", err)
			}
			request.Response.WriteStatus(500)
		})
	})
	server.Start()
	defer server.Shutdown()

	response, err := g.Client().Get(
		context.Background(),
		fmt.Sprintf("http://127.0.0.1:%d/problem", server.GetListenedPort()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	body := response.ReadAllString()
	if response.StatusCode != 500 || response.Header.Get("X-Trace-Id") != "" || response.Header.Get("Content-Type") == "application/problem+json" {
		t.Fatalf("response mutated before validation: status=%d content-type=%q trace=%q body=%q", response.StatusCode, response.Header.Get("Content-Type"), response.Header.Get("X-Trace-Id"), body)
	}
}

func TestWriterWritesImmutableMappedError(t *testing.T) {
	descriptor := problem.MustDescriptor(
		problem.MustKind("catalog.not_found", http.StatusNotFound),
		"https://errors.example.test/problems/catalog.not_found",
	)
	mapped, err := problem.NewError(descriptor, problem.Parameters{"id": "item-1"})
	if err != nil {
		t.Fatal(err)
	}
	writer := goframehttp.MustWriter(goframehttp.WriterOptions{})

	server := g.Server("foundation-mapped-error-writer")
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/problem", func(request *ghttp.Request) {
			ok, writeErr := writer.WriteError(request, mapped, "trace-mapped")
			if writeErr != nil || !ok {
				t.Errorf("WriteError() = %v, %v", ok, writeErr)
			}
		})
	})
	server.Start()
	defer server.Shutdown()

	response, err := g.Client().Get(
		context.Background(),
		fmt.Sprintf("http://127.0.0.1:%d/problem", server.GetListenedPort()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	value, err := problem.Decode(response.ReadAll())
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound || value.Code != "catalog.not_found" || value.TraceID != "trace-mapped" {
		t.Fatalf("status/value = %d/%#v", response.StatusCode, value)
	}
}
