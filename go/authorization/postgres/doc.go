// Package postgres provides the durable, consumer-local PostgreSQL Adapter,
// versioned schema generator, derived Casbin execution projection and offline
// protected-administrator recovery workflow for package authorization.
//
// Applications own migration execution. New validates the installed schema
// and catalog digest, bootstraps an absent instance explicitly, and otherwise
// restores domain truth without reinterpreting bootstrap configuration.
package postgres
