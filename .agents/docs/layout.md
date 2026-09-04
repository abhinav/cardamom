# Repository layout

Use this guide to locate the package that owns a change.
Read the owning package before adding another package or cross-package API.

| Path | Responsibility |
| --- | --- |
| `cmd/card` | Executable entry point. |
| `internal/cli` | Command syntax and output rendering. |
| `internal/board` | Board identity, state, settings, and finite board operations. |
| `internal/board/selection` | Board selection from explicit, ambient, checkout, or issue scope. |
| `internal/issue` | Shared issue identity, state, values, and read projections. |
| `internal/issue/planning` | Issue planning and graph changes. |
| `internal/issue/execution` | Eligibility, custody, lifecycle transitions, and checkpoint resolution. |
| `internal/issue/record` | Mutable notes, immutable comments, and durable results. |
| `internal/project` | Project identity, selection, and initialization contracts. |
| `internal/searchquery` | Public full-text search language parsing and validated expressions. |
| `internal/mail`, `lease`, `dump`, `information` | Domain-scoped product operations. |
| `internal/repository/store` | Store lifetime, migrations, and explicit read and write scopes. |
| `internal/repository/internal/query` | Generated low-level SQL operations shared only by repository implementations. |
| `internal/repository/{project,board,attachment,mail,lease,information}` | Owner-scoped persistence. |
| `internal/process` | Process lifetime and dependency composition. |
| `internal/storelocation` | Store discovery. |
| `internal/web` and domain Connect packages | Protocol translation and issue presentation. |
| `internal/web/server` | HTTP listener, embedded release assets, and live Vite process. |
| `protos/cardamom/v1` | Public CLI protobuf JSON schemas. |
| `protos/cardamom/private/v1` | Private browser protocol schemas. |
| `internal/gen/cardamom/{v1,private/v1}` | Generated public and private Go protocol code. |
| `web/src/gen/cardamom/{v1,private/v1}` | Generated public and private TypeScript protocol code. |
| `web` | React and Vite browser application. |
| `testdata/script` | Process-boundary CLI scenarios. |

## Frontend boundaries

`web/src/main.tsx` owns the React renderer and application-wide providers.
`web/src/app.tsx` owns the React Router route tree and application shell.

Use Connect Query with TanStack Query for RPC-backed server state.
Change notifications stream through the generated `ChangeClient`
and invalidate the shared `QueryClient` directly.

Attachment metadata remains query-backed.
Resumable uploads call the generated `AttachmentClient` directly
to sequence chunks, report progress, commit or abort the upload,
and then invalidate affected query state.

The principal dependency direction is:

```text
CLI and Connect service -> domain operations <- repository implementations
                                  ^
                                  |
                          process composition
```

The root `internal/repository` directory is organizational only;
it does not define a production package.
