// Package webhook provides instance-local, durable and signed webhook delivery.
//
// Product code publishes immutable CloudEvents. The module owns endpoint and
// subscription revisions, at-least-once delivery, Standard Webhooks signatures,
// attempt evidence, replay and inbound replay protection.
package webhook
