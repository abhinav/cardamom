# Search issue history

`card search` finds issue context when the issue or record that owns it is not
known yet.
Search is limited to the selected board and returns one result per matching
issue.
Each result identifies the fields that matched and shows the best excerpt.
When that excerpt comes from the Log, the result includes the stable Log ID for
an exact `card log show <log-id>` follow-up.

## Write a query

Adjacent terms require every term to occur in one searchable record.
Double quotes delimit a phrase, `OR` combines alternatives, infix `NOT`
excludes an expression, parentheses group expressions, and a trailing `*`
matches a word prefix.

```console
card search 'migration rollback'
card search '"schema version" OR migration*'
card search 'migration NOT legacy'
```

Use `--literal` when copied text may contain query operators or parentheses.
The complete argument is then matched as a phrase.

```console
card search --literal 'retry OR fall back'
```

Search covers Title, Summary, Details, State, Result, and Log by default.
Use `--in` to replace that scope with one or more fields.

```console
card search decision --in summary,details,result
card search timeout --in log
```

## Narrow the issue set

Search accepts the same containment, status, type, label, and custody filters
used for issue inspection.
Unlike `card list`, search includes terminal and non-terminal issues unless
`--status` narrows the result.

```console
card search 'storage format' --under card-plan
card search migration --status closed,cancelled --label area:database
card search timeout --assignee worker
```

Creation and closure filters use RFC 3339 times.
The `--*-since` boundaries are inclusive; the `--*-before` boundaries are
exclusive.
Closure filters include both closed and cancelled issues because both have a
closure time.
Add `--status closed` when cancelled issues must be excluded.

```console
card search migration \
  --created-since 2026-08-01T00:00:00Z \
  --created-before 2026-09-01T00:00:00Z
card search decision --status closed \
  --closed-since 2026-08-01T00:00:00Z
```

## Control and consume results

Results are ordered by relevance and limited to 20 issues by default.
The reported total is calculated before the limit, so it indicates when a
query should be narrowed or rerun with a larger limit.
`--limit 0` returns every match.
Non-relevance fields accepted by `card list --sort` are also available to
search, but `--reverse` is not meaningful with relevance order.

Human output is intended for discovery and follow-up:

```text
2 matches
ID       STATUS  MATCH       TITLE
card-17  closed  result,log  Choose the storage format
  log log_0123456789abcdef0123456789abcdef: ...[migration]...
```

Use `card show card-17 --context` to inspect the issue or
`card log show log_0123456789abcdef0123456789abcdef` to read that Log entry.
`--json` emits one object with `total` and `matches` members rather than JSON
Lines.
