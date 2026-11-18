# Write records readers can use

Expand Details with context, plans, constraints, or acceptance evidence too long
for Summary.
An investigation may need alternatives, measurement methods, and open questions.

## Show the current position

```bash
card --actor <actor> state set <id> - <<'STATE'
Search results now show file previews on expansion.
Keyboard navigation and empty-result checks pass; full validation is pending.
Next: run the full suite.
STATE
```

Put the next action in the body or `--next`, once.
Recheck facts that external changes may have invalidated.

## Preserve the choices worth remembering

Keep reasons that could prevent rework or change a later decision.
Include a reason in State too when continuation depends on it.

```bash
card --actor <actor> log post <id> - <<'LOG'
Load previews only for expanded results because loading every matching file
stalls large searches.
LOG
```

Update Summary or Details when the goal, constraints, or accepted approach changes.
Use `%<issue-id>` and `%log_<id>` for local references and full URLs for external
artifacts; state the relevant conclusion beside the reference.

## Describe what was delivered

```bash
card --actor <actor> result set <id> - <<'RESULT'
Search results show file previews on expansion.
Keyboard navigation, empty-result, and full-suite checks pass.
Slow network shares were not measured.
RESULT
```

State, Log, and Result accept `-` for standard input;
quote heredoc delimiters to preserve shell metacharacters.
For multiline Details, use a literal shell value:

```bash
details=$(cat <<'DETAILS'
Preserve the documented `$TARGET` and `\n` spellings.
DETAILS
)
card --actor <actor> edit <id> --details "$details"
```
