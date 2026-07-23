// Package postgres exposes the PostgreSQL Adapter and immutable schema
// generator for the URL Lifecycle Module.
package postgres

import (
	"context"

	"github.com/yueli-official/foundation/go/urllifecycle"
)

type Options = urllifecycle.PostgresOptions
type Adapter = urllifecycle.PostgresAdapter
type Migration = urllifecycle.PostgresMigration
type WrittenMigration = urllifecycle.WrittenPostgresMigration

const CurrentSchemaVersion = urllifecycle.CurrentPostgresSchemaVersion
const DefaultPrefix = urllifecycle.DefaultPostgresPrefix

func New(
	ctx context.Context,
	catalog *urllifecycle.Catalog,
	options Options,
) (*Adapter, error) {
	return urllifecycle.NewPostgres(ctx, catalog, options)
}

func Schema(version uint, prefix string) (Migration, error) {
	return urllifecycle.PostgresSchema(version, prefix)
}

func WriteMigration(
	directory, name string,
	version uint,
	prefix string,
) (WrittenMigration, error) {
	return urllifecycle.WritePostgresMigration(directory, name, version, prefix)
}
