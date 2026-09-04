# Features

- **Tasks with context**:
    Cardamom models work as a graph of issues.
    Parent issues contribute context to their children,
    so an agent sees all the relevant context of its task.
- **Record and replay**:
    Agents record decisions with Cardamom as they work.
    If an agent is lost, another agent can replay the history
    and end up roughly where the lost agent left off.
- **Full-text issue discovery**:
    Ranked board search spans issue plans, progress, outcomes, and Log history,
    with filters and excerpts that lead to focused follow-up reads.
- **Multiple sub-agents or processes**:
    Cardamom supports coordination of work across
    multiple sub-agents in the same process,
    or multiple agents in different processes,
    or a mixture of both.
- **Built-in web view**:
    The built-in web view provides a live view of issue state.
- **Flexible storage and boards**:
    Cardamom supports storing state on a per-user or per-repository basis.
    Within a store, your work can be organized into one board (per-repository)
    or multiple boards (e.g. per long-running effort).
- **Private by default**:
    Cardamom does not litter your repository.
    State is stored locally in a SQLite database.
    You may generate [Markdown collections](../guide/markdown-collections.md)
    to commit a report of your work, but the state itself is not shared.
- **Non-intrusive**:
    Cardamom does not require you to change your entire workflow,
    or use a completely different harness.
    It fits into your existing workflow, usable with your current harness.
    Install it as a skill, and use it to coordinate your agents.
