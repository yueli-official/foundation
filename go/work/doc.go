// Package work provides instance-local durable background work.
//
// Consumers own job kinds, JSON payloads and handlers. Adapters own durable
// enqueue, leasing, retry, scheduling and operational state.
package work
