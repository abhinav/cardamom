# Example workflow templates

Four templates showing different multi-agent shapes. Each step belongs
to an `agent` lane so multiple specialists can pick up work in
parallel without stepping on each other.

| File | Pattern | Lanes |
|---|---|---|
| `release.yaml` | linear handoff with one human gate | build-agent, test-agent, deploy-agent |
| `code-review.yaml` | fan-out (review ‖ tests) then converge | author-agent, reviewer-agent, test-agent, maintainer-agent |
| `research-spike.yaml` | wide fan-out across researchers, fan-in to synthesizer | planner-agent, researcher (×3), synthesizer-agent |
| `incident-response.yaml` | parallel kickoff with named exec sign-off | oncall-agent, investigator, fixer, comms-agent, deploy-agent, writer-agent |

## Quick start

```sh
# from the repo root
go build -o cli ./cmd/cli

# point at any directory you want as the working DB
mkdir -p /tmp/wf-demo && cd /tmp/wf-demo
/path/to/cli init

# either copy templates into the discovery dir…
mkdir -p .db/templates
cp /path/to/beadsv2/examples/workflows/*.yaml .db/templates/

# …or just point at a file (relative or absolute path also works)
/path/to/cli run /path/to/beadsv2/examples/workflows/code-review.yaml \
    --var pr=PR-42 --var author=alice --dry-run

# inspect
/path/to/cli template ls
/path/to/cli template validate code-review
/path/to/cli run code-review --var pr=PR-42 --var author=alice --dry-run
```

`cli run`, `cli template show`, and `cli template validate` all accept
either a template name (looked up in `.db/templates/`) **or** a path
ending in `.yaml`/`.yml`. The path form is handy for trying examples
without copying them first.

## Driving a workflow as multiple agents

Each agent process runs `cli claim -a <lane>` to grab work in its
own lane. Two terminals racing on the same lane is the supported
shape — `claim` is atomic.

```sh
# Instantiate a run.
cli run code-review --var pr=PR-42 --var author=alice

# Terminal A — the author agent.
cli claim -a author-agent --as author-1
cli close <draft-id>

# Terminals B and C in parallel — reviewer and test-runner.
cli claim -a reviewer-agent --as reviewer-1
cli claim -a test-agent --as ci-runner-1
# both terminals work, then:
cli close <review-id>
cli close <tests-id>

# Human gate.
cli approve <approve-id>

# Terminal D — maintainer agent.
cli claim -a maintainer-agent --as maint-1
cli close <merge-id>
```

`cli ready -a <lane>` shows what's available in a specific lane.
`cli list --label run:<parent-id>` shows everything from a single
instantiation.

## Notes

- **Checkpoints** (`type: checkpoint`) are issues that just wait.
  Clear them with `cli approve <id>` (manual or named approval) or
  `cli checkpoint fail <id> --reason "…"`.
- **Approvers** support variable interpolation: in
  `incident-response.yaml` the `exec-signoff` step has
  `wait: { approval: ["{{exec}}"] }` so each run binds it to the
  current on-call (`--var exec=$(id -un)`).
- The parent issue depends on every leaf step, so once all steps
  close the parent shows up in `cli ready` — a convenient signal that
  the whole run is done.
