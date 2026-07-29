# Observability

Cardamom exposes the same coordination state through CLI reads,
a local web interface,
and generated Markdown collections.
Choose the surface that matches the reader.

## CLI inspection

Read the work that is ready to begin:

```bash
card --actor observer ready
```

Read one issue with its board, ancestor,
and direct-dependency context:

```bash
card --actor observer show <issue-id> --context
```

Agents and scripts add `--json` when they need structured output.
For example, `ready` emits JSON Lines:

```bash
card --actor observer --json ready
```

## Local web interface

Start the browser interface from a checkout with a selected board:

```bash
card --actor observer web
```

The interface supports issue lists,
a board view,
issue editing,
log entries,
dependencies,
and checkpoint decisions.
It displays the board selected by the running `card web` process.

Use `--no-browser` when another process manages navigation
or when the server runs without a desktop session:

```bash
card --actor observer web --bind 127.0.0.1 --port 5757 --no-browser
```

## Review without a live process

Use a [Markdown collection](markdown-collections.md)
when reviewers need a deterministic,
browseable snapshot that can be committed or shared separately.
The collection is a one-way publication,
not a second writable Cardamom store.
