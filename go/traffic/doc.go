// Package traffic records privacy-bounded, instance-local resource views.
//
// A view is an observation that the consumer has already qualified against its
// product rules. The package separates event replay from daily visitor
// deduplication: replaying the same EventID never increments a counter twice,
// while a visitor may create multiple views in one day and contributes only
// once to that day's unique count.
//
// The package intentionally does not model sessions, funnels, conversions,
// arbitrary events, referrers, advertising attribution, or cross-day people.
package traffic
