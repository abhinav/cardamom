# Provide a constrained issue search language

Date: 2026-09-04
Status: Accepted

## Context

Cardamom issues hold planning and execution context across several records.
An agent that needs an older decision may not know which issue or record owns
that decision.
`card list --title-regexp` can filter known title patterns,
but it cannot discover text in Summary, Details, State, Result, or Log.

The search command must help an agent find a small set of likely issues.
Each result must identify the issue and the record that supplies its best
excerpt so the agent can continue with `card show` or `card log show`.
The command must remain useful when a board contains many issues and long Log
histories.

SQLite FTS5 has its own query syntax.
Exposing that syntax would couple the public CLI to the storage engine and
would expose operators that Cardamom does not intend to support.
Calling the command `grep` would also imply line-oriented regular-expression
matching, while Cardamom returns ranked issues and excerpts.

## Decision

Cardamom provides the board-scoped command `card search <query>`.
The command uses a small Cardamom-owned query language:

- Adjacent terms require every term.
- Double quotes delimit a phrase.
- `OR` combines alternatives.
- Infix `NOT` excludes a matching expression.
- Parentheses group expressions.
- A trailing `*` requests prefix matching.

`--literal` treats the complete query as a literal phrase.
`--in` limits search to one or more of `title`, `summary`, `details`, `state`,
`result`, and `log`.
Omitting `--in` searches every field.

Search applies issue metadata, containment, status, label, assignee,
creation-time, and closure-time filters.
Search does not define an update-time filter because an issue update timestamp
does not cover every standalone record change.
Search includes every issue status unless the caller supplies `--status`.

The default order is relevance and the default limit is 20 issues.
The command reports the total number of matching issues before the limit.
Each result includes the issue identity, title, status, matched fields,
and the best excerpt.
A Log excerpt also includes its stable Log ID.
Structured output is one JSON object with `total` and `matches` members.

## Consequences

Callers receive a stable search contract that does not expose SQLite FTS5
syntax or line-oriented matching behavior.
`card list --title-regexp` remains the command for regular-expression title
filtering.

The query parser must reject unsupported syntax rather than pass it through to
SQLite.
Future search operators require an intentional CLI contract change and a
translation to the storage query.

Bounded output is safe for agent context windows,
while the total count tells an agent when to refine the query or request more
results.
One result per issue prevents a long Log from flooding the output with several
matches from the same issue.
