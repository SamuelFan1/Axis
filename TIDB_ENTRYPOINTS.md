# Axis TiDB Entrypoints

Axis uses separate TiDB databases for core, derived, and runtime state.

## Target Rule

- Core data uses the authoritative TiDB entrypoint.
- Derived data uses the authoritative TiDB entrypoint.
- Runtime data uses the local regional TiDB entrypoint.

After the HAProxy migration, the standard SQL port is `4000` for both
authoritative and regional K8s HAProxy entries. The Docker-era `4406` and
`4416` ports remain legacy/fallback semantics only.

## Database Mapping

- `AXIS_CORE_DB_NAME=platform_core`
- `AXIS_DERIVED_DB_NAME=platform_derived`
- `AXIS_RUNTIME_DB_NAME=platform_runtime`

Recommended target shape:

```text
AXIS_CORE_DB_HOST=<authoritative-k8s-haproxy-host>
AXIS_CORE_DB_PORT=4000
AXIS_DERIVED_DB_HOST=<authoritative-k8s-haproxy-host>
AXIS_DERIVED_DB_PORT=4000
AXIS_RUNTIME_DB_HOST=<regional-k8s-haproxy-host>
AXIS_RUNTIME_DB_PORT=4000
```

Do not place `platform_runtime` into the global TiCDC mesh.
