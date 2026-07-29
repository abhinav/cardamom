# Contributing

Run development commands from the repository root.
Repository conventions are documented in [AGENTS.md](AGENTS.md).

## Install tools

Install the tool versions declared in `mise.toml`:

```bash
mise install
```

## Generate code

`protos/cardamom/v1` defines the public CLI protobuf JSON contract.
`protos/cardamom/private/v1` defines the browser-server protocol.
Generate the committed Go and TypeScript protocol code with:

```bash
mise run generate
```

Do not edit `internal/gen` or `web/src/gen` by hand.

The root `README.md` and top-level `USER_GUIDE.md` are also generated.
Edit `doc/readme/README.md`,
`doc/guide/README.md`,
and their linked source files,
then run:

```bash
mise run docs
```

## Run checks

Use the narrowest test that covers a change while iterating.
Before handing off completed work,
run the applicable checks:

```bash
mise run fmt
mise run lint
mise run test
mise run test:script
```

Run all Go tests with race detection using `mise run test --race`.
Run one process-boundary scenario with
`mise run test:script --run <name>`.

## Develop migrations safely

A branch binary that contains a migration not yet accepted into `main`
must not write the shared live coordination store.
Use a disposable `--store` until the migration is accepted into `main`
and the installed `card` binary is rebuilt from that accepted revision.

## Build for production

Build the frontend, generate the embedded web archive,
and compile `bin/card` with:

```bash
mise run build
```

The build installs the locked web dependencies under `web/`,
builds the React application with Vite,
generates `internal/web/server/static.tar.gz`,
and embeds the archive in the `card` binary with the `webassets` build tag.

To regenerate the archive from an existing `web/dist` directory without
installing dependencies or rebuilding the frontend,
run:

```bash
mise run web:archive
```

A source build without the `webassets` or `webdev` build tag remains usable
for non-web commands.
Its `card web` command reports that embedded web assets are unavailable.

Run the process scenario against embedded assets with:

```bash
mise run test:script:webassets
```

## Run live web development

Start `card` with Vite hot reload using:

```bash
mise run web:dev
```

Arguments after `--` are forwarded to `card web --dev`:

```bash
mise run web:dev -- --port 5758 --no-browser
```

Development mode starts Vite on a private loopback port and proxies browser
requests through the same public listener as the Connect handlers.
On Unix systems,
Vite runs in its own process group;
shutdown sends `SIGTERM` to the group and sends `SIGKILL` after the five-second
grace period if the group has not exited.
On Windows,
the operating-system process API does not provide the same graceful group
signal,
so shutdown kills the Vite child process immediately and waits for it to exit.

For source-only command iteration that does not need embedded assets,
run `go run ./cmd/card --help`.
