package webhook

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"
)

type scriptedResolver struct {
	mu      sync.Mutex
	answers [][]netip.Addr
	calls   int
}

func testAuthorizer(now time.Time) NetworkAuthorizer {
	return NetworkAuthorizer{
		Resolver: &scriptedResolver{answers: [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}}},
		Policy:   PublicNetworkPolicy(), Clock: func() time.Time { return now },
	}
}

func (resolver *scriptedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	index := min(resolver.calls, len(resolver.answers)-1)
	resolver.calls++
	return append([]netip.Addr(nil), resolver.answers[index]...), nil
}

func TestNetworkAuthorizerRejectsMixedAndReboundAnswers(t *testing.T) {
	resolver := &scriptedResolver{answers: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
	}}
	authorizer := NetworkAuthorizer{Resolver: resolver, Policy: PublicNetworkPolicy()}
	if _, err := authorizer.Authorize(context.Background(), "https://example.com/hook"); !IsCode(err, ErrorEndpointUnsafe) {
		t.Fatalf("mixed DNS answer err=%v", err)
	}
	resolver = &scriptedResolver{answers: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("10.0.0.10")},
	}}
	authorizer.Resolver = resolver
	if _, err := authorizer.Authorize(context.Background(), "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizer.Authorize(context.Background(), "https://example.com/hook"); !IsCode(err, ErrorEndpointUnsafe) {
		t.Fatalf("rebound DNS answer err=%v", err)
	}
}

func TestHTTPSenderPinsRouteAndDoesNotFollowRedirect(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits++
		http.Redirect(writer, request, "/other", http.StatusFound)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	address := netip.MustParseAddr(host)
	portNumber, _ := net.LookupPort("tcp", port)
	result, err := (HTTPSender{}).Send(context.Background(), OutboundRequest{
		Route: AuthorizedRoute{
			URL: parsed, Address: address, Port: uint16(portNumber), ExpiresAt: time.Now().Add(time.Minute),
		},
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusFound || hits != 1 {
		t.Fatalf("status=%d hits=%d", result.StatusCode, hits)
	}
}

func TestEndpointAdministrationHonorsExplicitDevelopmentPolicy(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	policy := PublicNetworkPolicy()
	policy.AllowHTTP = true
	policy.AllowLoopback = true
	policy.AllowedPorts = []uint16{8080}
	runtime, err := NewMemory(MustCompile(testDefinition()), MemoryOptions{
		Clock: func() time.Time { return now },
		Authorizer: NetworkAuthorizer{
			Policy: policy,
			Clock:  func() time.Time { return now },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.PutEndpoint(context.Background(), PutEndpointCommand{
		ID: "development", URL: "http://127.0.0.1:8080/hook",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Endpoint.URL != "http://127.0.0.1:8080/hook" {
		t.Fatalf("endpoint URL = %q", created.Endpoint.URL)
	}
}
