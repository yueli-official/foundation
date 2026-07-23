CREATE TABLE {{prefix}}state (
    id              SMALLINT PRIMARY KEY DEFAULT 1,
    revision        BIGINT NOT NULL,
    schema_version  BIGINT NOT NULL,
    document        JSONB NOT NULL,
    document_digest TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT {{prefix}}state_singleton CHECK (id = 1),
    CONSTRAINT {{prefix}}state_revision CHECK (revision > 0),
    CONSTRAINT {{prefix}}state_schema_version CHECK (schema_version > 0),
    CONSTRAINT {{prefix}}state_digest CHECK (document_digest ~ '^[0-9a-f]{64}$')
);
