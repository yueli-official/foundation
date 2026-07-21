# Architecture

Foundation is a polyglot repository with independently versioned Modules.

## Dependency direction

```text
contracts
  ├─ TypeScript core ── Vue/Nuxt UI Patterns ── Nuxt Adapters
  └─ ordinary Go core ───────────────────────── GoFrame Adapters
```

Core Modules cannot depend on framework Adapters. Product DTOs, endpoint topology, branding and deployment configuration remain with consumers.

## Promotion rule

A public Module must pass the deletion test, expose a smaller Interface than the complexity it hides, and be exercised through that same Interface by a packed-artifact conformance consumer. Preview-only usage is not promotion evidence.
