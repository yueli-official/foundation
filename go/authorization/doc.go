// Package authorization provides an instance-local authorization domain core.
//
// Consumers declare capabilities, scope topology, access layers and built-in
// roles, then compile that declaration into an immutable Catalog. The package
// does not authenticate subjects, own product resources, read process
// configuration or provide cross-instance authorization state.
package authorization
