# Capability

`capability` owns the framework-independent service capability manifest contract. A caller supplies
service metadata, capabilities, provider instances and presence-only configuration state;
`NewSnapshot` validates and normalizes the complete manifest into an immutable lookup/list snapshot.

The interface does not read process configuration, probe providers, authorize callers or expose
secret values. Service and transport adapters own those decisions and only publish the resulting
contract state.

The embedded JSON Schema retains the existing v1 contract identity so previously published
manifests remain wire-compatible.

验证：

```text
go test ./capability
```
