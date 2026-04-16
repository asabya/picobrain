# Migration: mandatory SpaCy startup

Picobrain now treats the local SpaCy parser sidecar as a required startup dependency.

## What changed

- `picobrain` no longer starts in a degraded mode when SpaCy is missing.
- Constructors that build a production `Brain` must initialize the parser successfully before returning.
- Commands that initialize a brain (`serve`, `export`, and `import`) now fail fast when SpaCy installation, process startup, or health checks fail.

## Removed config toggles

The optional startup toggles are removed:

- `EnableAutoGraph`
- `AutoInstallSpacy`

Automatic graph extraction is no longer guarded by an opt-in startup flag. Instead, Picobrain always requires a working parser.

## What stays configurable

- `SpacyCacheDir` remains only as a real install/discovery location override.
- `PICOBRAIN_SPACY_DIR` can point Picobrain at an existing SpaCy sidecar install.

Resolution order:

1. `PICOBRAIN_SPACY_DIR`
2. configured `SpacyCacheDir` / default `~/.picobrain/spacy`

## Operational impact

- First startup on a fresh machine may take longer because Picobrain may provision the SpaCy runtime before continuing.
- Startup now stops with an actionable error if install, process start, or health check steps fail.
- Existing environments with a valid SpaCy install should continue without reinstall churn.

## Migration checklist

1. Remove any references to `EnableAutoGraph` and `AutoInstallSpacy` from application config.
2. Ensure the host can create or access the SpaCy install directory.
3. If you manage the sidecar yourself, set `PICOBRAIN_SPACY_DIR` to the directory containing `server.py` and the virtualenv.
4. Re-run startup and confirm Picobrain reaches the MCP-ready banner before updating automation.

## Example Go config

```go
brain, err := picobrain.New(picobrain.Config{
    DBPath:        "~/.picobrain/brain.db",
    ModelCacheDir: "~/.picobrain/models",
    AutoDownload:  true,
    SpacyCacheDir: "~/.picobrain/spacy",
})
```

## Verification hints

- Fresh machine: verify Picobrain installs SpaCy and then starts successfully.
- Existing install: verify startup reuses the existing sidecar directory.
- Failure drills: verify install, startup, and health-check failures all abort startup with distinct messages.
