# Lazybase

Lazybase is a small Go wrapper around the Supabase CLI for people who run multiple local Supabase projects at once. It assigns each project a deterministic slot, rewrites only the supported host-port lines in `supabase/config.toml`, and gives you a tiny dashboard for registered projects.

## What problem it solves

Supabase projects default to the same local host ports. Running two projects side by side means manual port edits, port collisions, and repeated cleanup. Lazybase keeps that boring work deterministic:

- each project gets a stable slot
- each slot shifts the managed ports by a fixed offset
- `lazybase start` patches `supabase/config.toml` just before `supabase start`
- the original config text, comments, and formatting stay intact except for the targeted active port values

Lazybase targets the **current Supabase CLI config schema**, not an older PRD port list.

## Managed ports

Lazybase manages these current-schema keys:

- `api.port`
- `db.port`
- `db.shadow_port`
- `db.pooler.port`
- `studio.port`
- `inbucket.port`
- `inbucket.smtp_port` only when present and active
- `inbucket.pop3_port` only when present and active
- `analytics.port`
- `edge_runtime.inspector_port` only when present and active

Commented or missing optional keys are left untouched.

## Slot math

Lazybase uses:

- default offset: `100`
- computed port: `base + slot*offset`

Examples with the default offset:

- slot `0`: Studio `54323`, API `54321`
- slot `1`: Studio `54423`, API `54421`
- slot `2`: Studio `54523`, API `54521`

It allocates the lowest non-negative slot whose full managed port set is currently free.

## Files on disk

Lazybase resolves your home directory directly and stores its own files under `~/.config/lazybase/`.

- registry: `~/.config/lazybase/registry.json`
- optional global config: `~/.config/lazybase/lazybase.yaml`
- one-time config backup: `supabase/.config.toml.bak`

The backup is created only once, right before the first actual Lazybase modification to that project's `config.toml`.

## Global config

Optional file: `~/.config/lazybase/lazybase.yaml`

Currently supported:

```yaml
offset: 100
```

Base-port overrides are intentionally reserved for later to keep the first version simple.

## Commands

### Launch the dashboard

```bash
lazybase
```

The dashboard shows registered projects, slots, a computed port summary, status, and Studio URLs when available.

Keys:

- `enter` / `o`: open Studio for the selected project
- `p`: prune the selected registry entry
- `q`: quit

Pruning removes only the registry entry. It does not delete project files.

### Start a project with port assignment

```bash
lazybase start
lazybase start --debug
```

Flow:

1. ensure `supabase` exists in `PATH`
2. ensure the current directory has `supabase/config.toml`
3. validate the TOML
4. load global Lazybase config and registry
5. reuse or allocate a slot
6. patch active managed port lines in place
7. print a concise slot summary
8. run `supabase start ...`

### Pass-through commands

Everything except `start` is passed through, and when Lazybase can resolve the current project it injects `--workdir <runtime>` automatically (unless you already provided `--workdir`). Supabase CLI exit codes are preserved.

For safety, Lazybase skips runtime workdir injection for global/account-oriented commands (for example `projects`, `login`, `link`) and file-creation flows that should stay in your source tree (for example `migration new`, `db pull`).

When runtime workdir injection is active, Lazybase also links seed paths declared in `[db.seed].sql_paths` into runtime so `db reset` can resolve custom locations such as `./seeds/*.sql` and brace patterns like `./seeds/{core,dev}/*.sql`.

Valid seed path examples (relative to `supabase/`):

- `./seeds/*.sql`
- `./seeds/{core,dev}/*.sql`
- `./seed.sql`

Ignored seed path examples (with warning):

- `/tmp/seeds/*.sql` (absolute path)
- `../seeds/*.sql` (path traversal outside `supabase/`)

Example warning:

```text
Lazybase: warning: ignored db.seed.sql_paths entry "../seeds/*.sql": path traversal outside supabase dir is not allowed
```

```bash
lazybase stop
lazybase status
lazybase db reset
```

## Seamless aliasing

If you want Lazybase to front the Supabase CLI transparently:

```bash
alias supabase=lazybase
```

That works well for most commands because Lazybase only intercepts `start` specially and passes everything else through.

If your shell or workflow depends on shell functions instead of aliases, keep in mind that shell functions can shadow binaries differently across shells.

## Safety notes and limitations

- Lazybase only patches known, active port assignments in the current Supabase CLI config layout.
- It does **not** rewrite the whole TOML file, so comments and unrelated formatting are preserved.
- Port availability is currently checked with a simple local TCP listen probe. It is deterministic and intentionally simple.
- If `supabase status --workdir <project> -o json` is unavailable or unreliable, the dashboard falls back to a conservative status lookup.
- Lazybase is intentionally single-user and file-based for now.
