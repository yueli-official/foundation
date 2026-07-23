package abuse_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/yueli-official/foundation/go/abuse"
	"github.com/yueli-official/foundation/go/abuse/abusetest"
)

func TestPostgresConformance(t *testing.T) {
	database := openAbusePostgres(t)
	var instances atomic.Uint64
	abusetest.Run(t, func(
		t *testing.T,
		catalog *abuse.Catalog,
		clock func() time.Time,
		verifiers map[abuse.ChallengeKind]abuse.ChallengeVerifier,
	) abuse.Module {
		t.Helper()
		module, err := abuse.NewPostgres(context.Background(), catalog, abuse.PostgresOptions{
			DB: database, InstanceKey: fmt.Sprintf("conformance-%d", instances.Add(1)),
			Clock: clock, Secret: []byte("postgres-conformance-secret-32-bytes"),
			Verifiers: verifiers,
		})
		if err != nil {
			t.Fatal(err)
		}
		return module
	})
}

func TestPostgresConcurrentAdmissionIsExact(t *testing.T) {
	database := openAbusePostgres(t)
	catalog := abuse.MustCompile(abuse.Definition{
		Version: 1, Consumer: "concurrency",
		Actions: []abuse.ActionDefinition{{
			Key:      "test.concurrent",
			Required: abuse.SignalRequirements{Network: abuse.Required},
			Meters: []abuse.MeterDefinition{{
				ID: "concurrent.network", Slot: abuse.SlotNetwork,
				Algorithm: abuse.FixedWindow(10, time.Hour),
			}},
		}},
	})
	module, err := abuse.NewPostgres(context.Background(), catalog, abuse.PostgresOptions{
		DB: database, InstanceKey: "concurrency",
		Secret: []byte("postgres-concurrency-secret-32-bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	action, err := module.Action("test.concurrent")
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 50
	start := make(chan struct{})
	results := make(chan abuse.Disposition, attempts)
	errorsChannel := make(chan error, attempts)
	var wait sync.WaitGroup
	for index := 0; index < attempts; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			admission, err := action.Admit(context.Background(), abuse.Input{
				ID: abuse.AttemptID(fmt.Sprintf("concurrent-%06d", index)),
				Signals: abuse.Signals{
					Network: netip.MustParsePrefix("203.0.113.8/32"),
				},
			})
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- admission.Disposition
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent admission: %v", err)
	}
	allowed, rejected := 0, 0
	for result := range results {
		switch result {
		case abuse.DispositionAllow:
			allowed++
		case abuse.DispositionReject:
			rejected++
		default:
			t.Fatalf("unexpected disposition %q", result)
		}
	}
	if allowed != 10 || rejected != attempts-10 {
		t.Fatalf("allowed=%d rejected=%d", allowed, rejected)
	}
}

func TestPostgresCompatiblePolicyUpgradeAndPrivacy(t *testing.T) {
	database := openAbusePostgres(t)
	definition := func(version, policyRevision uint64, capacity int64) abuse.Definition {
		return abuse.Definition{
			Version: version, Consumer: "upgrade",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.upgrade",
				Required: abuse.SignalRequirements{Target: abuse.Required},
				Meters: []abuse.MeterDefinition{{
					ID: "upgrade.target", Revision: policyRevision, Slot: abuse.SlotTarget,
					Algorithm: abuse.FixedWindow(capacity, time.Hour),
				}},
			}},
		}
	}
	first, err := abuse.NewPostgres(context.Background(), abuse.MustCompile(definition(1, 1, 2)), abuse.PostgresOptions{
		DB: database, InstanceKey: "upgrade", Secret: []byte("first-postgres-secret-that-is-long-enough"),
	})
	if err != nil {
		t.Fatal(err)
	}
	action, _ := first.Action("test.upgrade")
	signals := abuse.Signals{
		Network: netip.MustParsePrefix("198.51.100.32/32"),
		Target:  "private@example.test",
	}
	if admission, err := action.Admit(context.Background(), abuse.Input{
		ID: "upgrade-attempt-001", Signals: signals,
	}); err != nil || admission.Disposition != abuse.DispositionAllow {
		t.Fatalf("first admission=%+v err=%v", admission, err)
	}
	before, err := first.Governor().Inspect(context.Background(), abuse.InspectQuery{
		Action: "test.upgrade", Signals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := abuse.NewPostgres(context.Background(), abuse.MustCompile(definition(2, 2, 4)), abuse.PostgresOptions{
		DB: database, InstanceKey: "upgrade", Secret: []byte("different-secret-is-ignored-after-restart"),
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := upgraded.Governor().Inspect(context.Background(), abuse.InspectQuery{
		Action: "test.upgrade", Signals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	if before.Meters[0].SubjectRef != after.Meters[0].SubjectRef || after.Meters[0].Used != 1 || after.Meters[0].Capacity != 4 {
		t.Fatalf("upgrade did not preserve state/key: before=%+v after=%+v", before, after)
	}

	var durable string
	if err := database.QueryRow(`
SELECT state::text || ' ' || encode(subject_key, 'hex')
FROM abuse_meter_states
WHERE instance_key='upgrade' AND policy_id='upgrade.target'`).Scan(&durable); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(durable, "private@example.test") || strings.Contains(durable, "198.51.100.32") {
		t.Fatalf("durable state leaked a raw signal: %s", durable)
	}

	changedAlgorithm := definition(3, 3, 4)
	changedAlgorithm.Actions[0].Meters[0].Algorithm = abuse.SlidingWindow(4, time.Hour)
	if _, err := abuse.NewPostgres(context.Background(), abuse.MustCompile(changedAlgorithm), abuse.PostgresOptions{
		DB: database, InstanceKey: "upgrade",
	}); !abuse.IsKind(err, abuse.ErrorDefinitionDrift) {
		t.Fatalf("algorithm change under the same policy id must fail, got %v", err)
	}
}

func openAbusePostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ABUSE_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("TRAFFIC_POSTGRES_DSN")
	}
	if dsn == "" {
		dsn = os.Getenv("AUTHORIZATION_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("ABUSE_POSTGRES_DSN is not configured")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("abuse_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName)); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	scoped, err := abuseSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("postgres", scoped)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(64)
	if err := database.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(abuse.PostgresSchemaUp()); err != nil {
		t.Fatalf("schema up: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(abuse.PostgresSchemaDown())
		_ = database.Close()
		_, _ = admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName) + " CASCADE")
		_ = admin.Close()
	})
	return database
}

func abuseSearchPath(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "postgres" || parsed.Scheme == "postgresql" {
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	return dsn + " search_path=" + pq.QuoteIdentifier(schema), nil
}
