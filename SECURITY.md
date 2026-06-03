# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
pull requests, or discussions.**

Instead, use **[GitHub's private vulnerability reporting](https://github.com/arjia-labs/clu/security/advisories/new)**
("Report a vulnerability" under the repository's **Security** tab). This opens a
private advisory visible only to you and the maintainers.

If you can't use that, email **rovak@proton.me** with the details.

Please include:

- A description of the issue and its impact.
- Steps to reproduce (a minimal proof of concept is ideal).
- The `clu` version (`clu version`) and your OS.

We'll acknowledge your report as quickly as we can, keep you updated on the
fix, and credit you in the advisory unless you'd prefer to remain anonymous.
Please give us a reasonable window to ship a fix before any public disclosure.

## Supported versions

`clu` is pre-1.0 and moves quickly. Security fixes land on `main` and in the
next tagged release.

| Version | Supported |
| --- | --- |
| Latest release & `main` | ✅ |
| Older releases | ❌ — please upgrade |

## Scope

`clu` is a **local-first** tool: it runs on your machine against a local SQLite
database, with no network, accounts, or telemetry. Keep that model in mind when
assessing severity.

In scope — please report:

- Memory-safety or data-corruption bugs reachable from untrusted input
  (e.g. a malicious `clu import` / `clu batch` JSONL document or a crafted
  workflow template).
- Injection or path-traversal issues (SQL, command, or file paths) reachable
  without already having local code execution.
- Privilege escalation or sandbox escape beyond the invoking user's existing
  access.

Out of scope (these are intentional design choices, not vulnerabilities):

- **`clu sql --write`** runs arbitrary SQL against your own database — by design.
- **`clu http` / `clu web`** start an **unauthenticated** server intended to
  bind to localhost for a single user. Don't expose it to untrusted networks.
- **`clu cron`, `clu agent start`, and workflow templates** execute commands
  *you* configure — running your own configured commands is the feature, not a flaw.
- Anything requiring an attacker to already have local shell access as your user.

When in doubt, report it privately and we'll figure out severity together.
