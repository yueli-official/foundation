CREATE TABLE {{prefix}}instances (
    instance_key TEXT PRIMARY KEY,
    schema_version BIGINT NOT NULL,
    catalog_version BIGINT NOT NULL,
    catalog_digest TEXT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE {{prefix}}routes (
    instance_key TEXT NOT NULL REFERENCES {{prefix}}instances(instance_key) ON DELETE CASCADE,
    route_key TEXT NOT NULL,
    resource_kind TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    variant TEXT NOT NULL DEFAULT '',
    canonical_ref_id TEXT NOT NULL,
    route_revision BIGINT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, route_key),
    UNIQUE (instance_key, resource_kind, resource_id, variant)
);

CREATE TABLE {{prefix}}references (
    instance_key TEXT NOT NULL REFERENCES {{prefix}}instances(instance_key) ON DELETE CASCADE,
    ref_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    path TEXT NOT NULL,
    identity_query TEXT NOT NULL DEFAULT '',
    rendered_query TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    owner_kind TEXT NOT NULL DEFAULT '',
    owner_id TEXT NOT NULL DEFAULT '',
    owner_variant TEXT NOT NULL DEFAULT '',
    target_kind TEXT NOT NULL DEFAULT '',
    target_route_kind TEXT NOT NULL DEFAULT '',
    target_route_id TEXT NOT NULL DEFAULT '',
    target_route_variant TEXT NOT NULL DEFAULT '',
    target_external TEXT NOT NULL DEFAULT '',
    redirect_mode TEXT NOT NULL DEFAULT '',
    query_mode TEXT NOT NULL DEFAULT '',
    replace_query TEXT NOT NULL DEFAULT '',
    changed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, ref_id),
    UNIQUE (instance_key, path, namespace, identity_query)
);
CREATE INDEX {{prefix}}references_owner_idx
    ON {{prefix}}references (instance_key, owner_kind, owner_id, owner_variant);
CREATE INDEX {{prefix}}references_target_idx
    ON {{prefix}}references (
        instance_key, target_route_kind, target_route_id, target_route_variant
    );

CREATE TABLE {{prefix}}overlays (
    instance_key TEXT NOT NULL,
    ref_id TEXT NOT NULL,
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    owner_variant TEXT NOT NULL DEFAULT '',
    target_kind TEXT NOT NULL,
    target_route_kind TEXT NOT NULL DEFAULT '',
    target_route_id TEXT NOT NULL DEFAULT '',
    target_route_variant TEXT NOT NULL DEFAULT '',
    target_external TEXT NOT NULL DEFAULT '',
    redirect_mode TEXT NOT NULL,
    query_mode TEXT NOT NULL,
    replace_query TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    changed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, ref_id),
    FOREIGN KEY (instance_key, ref_id)
        REFERENCES {{prefix}}references(instance_key, ref_id) ON DELETE CASCADE
);

CREATE TABLE {{prefix}}commands (
    instance_key TEXT NOT NULL REFERENCES {{prefix}}instances(instance_key) ON DELETE CASCADE,
    command_id TEXT NOT NULL,
    intent_digest TEXT NOT NULL,
    receipt JSONB NOT NULL,
    PRIMARY KEY (instance_key, command_id)
);

CREATE TABLE {{prefix}}history (
    instance_key TEXT NOT NULL REFERENCES {{prefix}}instances(instance_key) ON DELETE CASCADE,
    revision BIGINT NOT NULL,
    command_id TEXT NOT NULL,
    actor_kind TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, revision)
);
