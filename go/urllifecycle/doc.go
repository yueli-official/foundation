// Package urllifecycle owns instance-local public URL claims, aliases,
// redirects, gone outcomes, temporary overlays, atomic transitions, and
// deterministic resolution.
//
// Product code owns entities, publication state, authorization, and the reason
// for a URL change. Discovery consumes committed canonical claims but is not a
// URL lifecycle source of truth.
package urllifecycle
