package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type PostgresOptions struct {
	DB          *sql.DB
	InstanceKey string
	Clock       func() time.Time
	Scheduler   TransactionalScheduler
	Secrets     SecretStore
	Authorizer  NetworkAuthorizer
}

type Postgres struct {
	db          *sql.DB
	tx          *sql.Tx
	instanceKey string
	catalog     *Catalog
	clock       func() time.Time
	scheduler   TransactionalScheduler
	secrets     SecretStore
	authorizer  NetworkAuthorizer
}

var _ Runtime = (*Postgres)(nil)

func NewPostgres(ctx context.Context, catalog *Catalog, options PostgresOptions) (*Postgres, error) {
	if catalog == nil {
		return nil, invalid(ErrorInvalidDefinition, "catalog", "is required")
	}
	if options.DB == nil {
		return nil, invalid(ErrorInvalidDefinition, "db", "is required")
	}
	if options.Scheduler == nil {
		return nil, invalid(ErrorInvalidDefinition, "scheduler", "is required")
	}
	if options.Secrets == nil {
		return nil, invalid(ErrorInvalidDefinition, "secrets", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" || len(instanceKey) > 200 || strings.ContainsRune(instanceKey, '\x00') {
		return nil, invalid(ErrorInvalidDefinition, "instance_key", "is invalid")
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, unavailable("ping database", err)
	}
	adapter := &Postgres{
		db: options.DB, instanceKey: instanceKey, catalog: catalog, clock: options.Clock,
		scheduler: options.Scheduler, secrets: options.Secrets, authorizer: options.Authorizer,
	}
	if err := adapter.bootstrap(ctx); err != nil {
		return nil, err
	}
	return adapter, nil
}

// Bind joins Publish and Accept to a caller-owned transaction. The adapter
// never commits or rolls back tx.
func (adapter *Postgres) Bind(tx *sql.Tx) *Postgres {
	if adapter == nil || tx == nil {
		return nil
	}
	copy := *adapter
	copy.tx = tx
	return &copy
}

func (adapter *Postgres) bootstrap(ctx context.Context) error {
	now := adapter.clock().UTC()
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_instances(instance_key,schema_version,catalog_version,catalog_digest,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$5)
ON CONFLICT (instance_key) DO NOTHING`,
		adapter.instanceKey, CurrentSchemaVersion, adapter.catalog.version, adapter.catalog.digest, now,
	); err != nil {
		return unavailable("bootstrap instance", err)
	}
	var schemaVersion, catalogVersion int64
	var digest string
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version,catalog_version,catalog_digest
FROM webhook_instances WHERE instance_key=$1 FOR UPDATE`,
		adapter.instanceKey,
	).Scan(&schemaVersion, &catalogVersion, &digest); err != nil {
		return unavailable("load instance", err)
	}
	if schemaVersion != int64(CurrentSchemaVersion) {
		return &Error{Code: ErrorInvalidDefinition, Field: "schema_version", Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, CurrentSchemaVersion)}
	}
	if catalogVersion != int64(adapter.catalog.version) || digest != adapter.catalog.digest {
		return &Error{Code: ErrorInvalidDefinition, Field: "catalog", Message: "persisted catalog version or digest differs"}
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit bootstrap", err)
	}
	return nil
}

func (adapter *Postgres) Publish(ctx context.Context, command EventCommand) (EventReceipt, error) {
	if adapter.tx != nil {
		return adapter.publishTx(ctx, adapter.tx, command)
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return EventReceipt{}, unavailable("begin publish", err)
	}
	defer func() { _ = tx.Rollback() }()
	receipt, err := adapter.publishTx(ctx, tx, command)
	if err != nil {
		return EventReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return EventReceipt{}, unavailable("commit publish", err)
	}
	return receipt, nil
}

func (adapter *Postgres) PublishTx(ctx context.Context, tx *sql.Tx, command EventCommand) (EventReceipt, error) {
	if tx == nil {
		return EventReceipt{}, invalid(ErrorInvalidEvent, "tx", "is required")
	}
	return adapter.publishTx(ctx, tx, command)
}

func (adapter *Postgres) publishTx(ctx context.Context, tx *sql.Tx, command EventCommand) (EventReceipt, error) {
	now := adapter.clock().UTC()
	prepared, err := prepareEvent(adapter.catalog, now, command)
	if err != nil {
		return EventReceipt{}, err
	}
	var existing EventReceipt
	var fingerprint string
	err = tx.QueryRowContext(ctx, `
SELECT event_id,body_digest,published_at,command_fingerprint,
       (SELECT count(*) FROM webhook_deliveries d WHERE d.instance_key=e.instance_key AND d.event_id=e.event_id)
FROM webhook_events e WHERE instance_key=$1 AND idempotency_key=$2`,
		adapter.instanceKey, command.IdempotencyKey,
	).Scan(&existing.EventID, &existing.BodyDigest, &existing.PublishedAt, &fingerprint, &existing.Deliveries)
	switch {
	case err == nil:
		if fingerprint != prepared.fingerprint {
			return EventReceipt{}, &Error{Code: ErrorIdempotency, Field: "idempotency_key", Message: "was used for different event"}
		}
		existing.Duplicate = true
		return existing, nil
	case err != sql.ErrNoRows:
		return EventReceipt{}, unavailable("lookup event idempotency", err)
	}
	idText, err := NewID()
	if err != nil {
		return EventReceipt{}, err
	}
	eventID := EventID(idText)
	body, err := encodeCloudEvent(adapter.catalog, eventID, command)
	if err != nil {
		return EventReceipt{}, unavailable("encode CloudEvent", err)
	}
	if len(body) > adapter.catalog.limits.MaxEventBytes {
		return EventReceipt{}, invalid(ErrorEventTooLarge, "event", "encoded CloudEvent exceeds limit")
	}
	bodyDigest := digestBytes(body)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_events(
 instance_key,event_id,event_type,subject,raw_body,body_digest,idempotency_key,
 command_fingerprint,occurred_at,published_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		adapter.instanceKey, eventID, command.Type, strings.TrimSpace(command.Subject), body,
		bodyDigest, strings.TrimSpace(command.IdempotencyKey), prepared.fingerprint,
		command.OccurredAt.UTC(), now,
	); err != nil {
		return EventReceipt{}, unavailable("insert event", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT s.subscription_id,s.current_revision,s.endpoint_id,e.current_revision
FROM webhook_subscriptions s
JOIN webhook_subscription_revisions sr
  ON sr.instance_key=s.instance_key AND sr.subscription_id=s.subscription_id AND sr.revision=s.current_revision
JOIN webhook_endpoints e
  ON e.instance_key=s.instance_key AND e.endpoint_id=s.endpoint_id
WHERE s.instance_key=$1 AND s.enabled=TRUE AND e.current_state='active'
  AND sr.event_types ? $2
ORDER BY s.subscription_id
LIMIT $3`,
		adapter.instanceKey, command.Type, adapter.catalog.limits.MaxFanout+1,
	)
	if err != nil {
		return EventReceipt{}, unavailable("select subscriptions", err)
	}
	type routeSnapshot struct {
		subscriptionID       SubscriptionID
		subscriptionRevision uint64
		endpointID           EndpointID
		endpointRevision     uint64
	}
	routes := make([]routeSnapshot, 0)
	for rows.Next() {
		var route routeSnapshot
		if err := rows.Scan(
			&route.subscriptionID,
			&route.subscriptionRevision,
			&route.endpointID,
			&route.endpointRevision,
		); err != nil {
			_ = rows.Close()
			return EventReceipt{}, unavailable("scan subscription", err)
		}
		routes = append(routes, route)
		if len(routes) > adapter.catalog.limits.MaxFanout {
			_ = rows.Close()
			return EventReceipt{}, invalid(ErrorLimitExceeded, "fanout", "exceeds configured maximum")
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return EventReceipt{}, unavailable("iterate subscriptions", err)
	}
	if err := rows.Close(); err != nil {
		return EventReceipt{}, unavailable("close subscriptions", err)
	}
	for _, route := range routes {
		deliveryText, err := NewID()
		if err != nil {
			return EventReceipt{}, err
		}
		deliveryID := DeliveryID(deliveryText)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_deliveries(
 instance_key,delivery_id,event_id,endpoint_id,endpoint_revision,subscription_id,
 subscription_revision,state,attempt_count,next_attempt_at,created_at,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',0,$8,$8,$8)`,
			adapter.instanceKey, deliveryID, eventID, route.endpointID, route.endpointRevision,
			route.subscriptionID, route.subscriptionRevision, now,
		); err != nil {
			return EventReceipt{}, unavailable("insert delivery", err)
		}
		if err := adapter.scheduler.EnqueueTx(ctx, tx, DeliveryWork{
			DeliveryID: deliveryID, RunAt: now, Key: "webhook.delivery:" + string(deliveryID),
		}); err != nil {
			return EventReceipt{}, err
		}
	}
	return EventReceipt{
		EventID: eventID, Deliveries: len(routes), BodyDigest: bodyDigest, PublishedAt: now,
	}, nil
}

func (adapter *Postgres) PutEndpoint(ctx context.Context, command PutEndpointCommand) (EndpointCredential, error) {
	var orphanRef SecretRef
	var orphanRevision SecretRevision
	defer func() {
		if orphanRef == "" {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adapter.secrets.Delete(cleanupContext, orphanRef, orphanRevision)
	}()
	route, err := adapter.authorizer.Authorize(ctx, command.URL)
	if err != nil {
		return EndpointCredential{}, err
	}
	canonical := route.URL.String()
	if len(command.Description) > adapter.catalog.limits.MaxDescriptionBytes {
		return EndpointCredential{}, invalid(ErrorLimitExceeded, "description", "is too large")
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return EndpointCredential{}, unavailable("begin endpoint update", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := adapter.clock().UTC()
	id := command.ID
	var existing Endpoint
	var secretRef SecretRef
	err = tx.QueryRowContext(ctx, `
SELECT e.endpoint_id,e.current_revision,r.target_url,r.description,e.current_state,e.etag,e.secret_ref,e.created_at,e.updated_at
FROM webhook_endpoints e
JOIN webhook_endpoint_revisions r
  ON r.instance_key=e.instance_key AND r.endpoint_id=e.endpoint_id AND r.revision=e.current_revision
WHERE e.instance_key=$1 AND e.endpoint_id=$2 FOR UPDATE`,
		adapter.instanceKey, id,
	).Scan(&existing.ID, &existing.Revision, &existing.URL, &existing.Description,
		&existing.State, &existing.ETag, &secretRef, &existing.CreatedAt, &existing.UpdatedAt)
	exists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return EndpointCredential{}, unavailable("load endpoint", err)
	}
	if exists && command.ExpectedETag != existing.ETag {
		return EndpointCredential{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	if !exists {
		if id == "" {
			text, generateErr := NewID()
			if generateErr != nil {
				return EndpointCredential{}, generateErr
			}
			id = EndpointID(text)
		} else if !stableKey.MatchString(string(id)) {
			return EndpointCredential{}, invalid(ErrorInvalidEvent, "endpoint_id", "is invalid")
		}
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM webhook_endpoints WHERE instance_key=$1`, adapter.instanceKey).Scan(&total); err != nil {
			return EndpointCredential{}, unavailable("count endpoints", err)
		}
		if total >= adapter.catalog.limits.MaxEndpoints {
			return EndpointCredential{}, invalid(ErrorLimitExceeded, "endpoints", "limit reached")
		}
	}
	revision, createdAt, state := uint64(1), now, EndpointActive
	var plaintext string
	if exists {
		revision, createdAt, state = existing.Revision+1, existing.CreatedAt, existing.State
	} else {
		secretRef = SecretRef("webhook.endpoint." + string(id))
		plaintext, err = newSecret()
		if err != nil {
			return EndpointCredential{}, err
		}
		decoded, _ := decodeSecret(plaintext)
		if err := adapter.secrets.Create(ctx, secretRef, SecretMaterial{
			Revision: "r1", Value: decoded, NotBefore: now,
		}); err != nil {
			return EndpointCredential{}, err
		}
		orphanRef, orphanRevision = secretRef, "r1"
	}
	endpoint := Endpoint{
		ID: id, Revision: revision, URL: canonical, Description: strings.TrimSpace(command.Description),
		State: state, CreatedAt: createdAt, UpdatedAt: now,
	}
	endpoint.ETag = endpointETag(endpoint)
	if exists {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_endpoints SET current_revision=$3,current_state=$4,etag=$5,updated_at=$6
WHERE instance_key=$1 AND endpoint_id=$2`,
			adapter.instanceKey, id, revision, state, endpoint.ETag, now,
		); err != nil {
			return EndpointCredential{}, unavailable("update endpoint", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_endpoints(
 instance_key,endpoint_id,current_revision,current_state,secret_ref,etag,created_at,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`,
			adapter.instanceKey, id, revision, state, secretRef, endpoint.ETag, now,
		); err != nil {
			return EndpointCredential{}, unavailable("insert endpoint", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_endpoint_revisions(
 instance_key,endpoint_id,revision,target_url,description,state,etag,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		adapter.instanceKey, id, revision, canonical, endpoint.Description, state, endpoint.ETag, now,
	); err != nil {
		return EndpointCredential{}, unavailable("insert endpoint revision", err)
	}
	if err := tx.Commit(); err != nil {
		return EndpointCredential{}, unavailable("commit endpoint", err)
	}
	orphanRef, orphanRevision = "", ""
	return EndpointCredential{Endpoint: endpoint, Secret: plaintext}, nil
}

func (adapter *Postgres) PutSubscription(ctx context.Context, command PutSubscriptionCommand) (Subscription, error) {
	if len(command.EventTypes) == 0 || len(command.EventTypes) > adapter.catalog.limits.MaxEventTypesPerSub {
		return Subscription{}, invalid(ErrorLimitExceeded, "event_types", "count is invalid")
	}
	types := append([]EventType(nil), command.EventTypes...)
	slices.Sort(types)
	types = slices.Compact(types)
	for _, eventType := range types {
		if _, exists := adapter.catalog.eventTypes[eventType]; !exists {
			return Subscription{}, invalid(ErrorInvalidEvent, "event_types", fmt.Sprintf("%q is not registered", eventType))
		}
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, unavailable("begin subscription update", err)
	}
	defer func() { _ = tx.Rollback() }()
	var endpointExists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM webhook_endpoints WHERE instance_key=$1 AND endpoint_id=$2)`,
		adapter.instanceKey, command.EndpointID,
	).Scan(&endpointExists); err != nil {
		return Subscription{}, unavailable("check endpoint", err)
	}
	if !endpointExists {
		return Subscription{}, &Error{Code: ErrorNotFound, Field: "endpoint_id", Message: "does not exist"}
	}
	now := adapter.clock().UTC()
	id := command.ID
	var existing Subscription
	err = tx.QueryRowContext(ctx, `
SELECT s.subscription_id,s.current_revision,s.endpoint_id,r.event_types,s.enabled,r.description,s.etag,s.created_at,s.updated_at
FROM webhook_subscriptions s
JOIN webhook_subscription_revisions r
  ON r.instance_key=s.instance_key AND r.subscription_id=s.subscription_id AND r.revision=s.current_revision
WHERE s.instance_key=$1 AND s.subscription_id=$2 FOR UPDATE`,
		adapter.instanceKey, id,
	).Scan(&existing.ID, &existing.Revision, &existing.EndpointID, pqEventTypes(&existing.EventTypes),
		&existing.Enabled, &existing.Description, &existing.ETag, &existing.CreatedAt, &existing.UpdatedAt)
	exists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return Subscription{}, unavailable("load subscription", err)
	}
	if exists && command.ExpectedETag != existing.ETag {
		return Subscription{}, &Error{Code: ErrorETagConflict, Field: "expected_etag", Message: "does not match"}
	}
	if !exists {
		if id == "" {
			text, generateErr := NewID()
			if generateErr != nil {
				return Subscription{}, generateErr
			}
			id = SubscriptionID(text)
		} else if !stableKey.MatchString(string(id)) {
			return Subscription{}, invalid(ErrorInvalidEvent, "subscription_id", "is invalid")
		}
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM webhook_subscriptions WHERE instance_key=$1`, adapter.instanceKey).Scan(&total); err != nil {
			return Subscription{}, unavailable("count subscriptions", err)
		}
		if total >= adapter.catalog.limits.MaxSubscriptions {
			return Subscription{}, invalid(ErrorLimitExceeded, "subscriptions", "limit reached")
		}
	}
	revision, createdAt := uint64(1), now
	if exists {
		revision, createdAt = existing.Revision+1, existing.CreatedAt
	}
	subscription := Subscription{
		ID: id, Revision: revision, EndpointID: command.EndpointID, EventTypes: types,
		Enabled: command.Enabled, Description: strings.TrimSpace(command.Description),
		CreatedAt: createdAt, UpdatedAt: now,
	}
	subscription.ETag = subscriptionETag(subscription)
	encodedTypes, _ := json.Marshal(types)
	if exists {
		if _, err := tx.ExecContext(ctx, `
UPDATE webhook_subscriptions SET endpoint_id=$3,current_revision=$4,enabled=$5,etag=$6,updated_at=$7
WHERE instance_key=$1 AND subscription_id=$2`,
			adapter.instanceKey, id, command.EndpointID, revision, command.Enabled, subscription.ETag, now,
		); err != nil {
			return Subscription{}, unavailable("update subscription", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_subscriptions(
 instance_key,subscription_id,endpoint_id,current_revision,enabled,etag,created_at,updated_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`,
			adapter.instanceKey, id, command.EndpointID, revision, command.Enabled, subscription.ETag, now,
		); err != nil {
			return Subscription{}, unavailable("insert subscription", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_subscription_revisions(
 instance_key,subscription_id,revision,endpoint_id,event_types,enabled,description,etag,created_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		adapter.instanceKey, id, revision, command.EndpointID, encodedTypes, command.Enabled,
		subscription.Description, subscription.ETag, now,
	); err != nil {
		return Subscription{}, unavailable("insert subscription revision", err)
	}
	if err := tx.Commit(); err != nil {
		return Subscription{}, unavailable("commit subscription", err)
	}
	return subscription, nil
}

type jsonEventTypesScanner struct{ target *[]EventType }

func pqEventTypes(target *[]EventType) *jsonEventTypesScanner {
	return &jsonEventTypesScanner{target: target}
}
func (scanner *jsonEventTypesScanner) Scan(source any) error {
	var data []byte
	switch value := source.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return fmt.Errorf("webhook: unsupported event types source %T", source)
	}
	return json.Unmarshal(data, scanner.target)
}
