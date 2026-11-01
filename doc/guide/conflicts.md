# Inspect and resolve conflicts

A disputed field has no current value.
Cardamom keeps its alternatives and identifies the conflict in issue and board views.
Other fields remain readable and editable.
Automatic claiming skips affected work and reports how many issues it skipped;
explicit operations require only the resolved inputs they use.

List conflicts for the selected board or one issue:

```bash
card conflicts
card conflicts list %cm-a1b2
```

The list includes conflict identifiers and continuation guidance.
Use `--json` for structured output, and `--after` to continue a page.
Inspection lists alternative identifiers without loading every body.
Read the alternatives you need before selecting one:

```bash
card conflicts show CONFLICT
card conflicts show CONFLICT --alternative ALTERNATIVE
card resolve CONFLICT --take ALTERNATIVE
```

Replace `CONFLICT` and `ALTERNATIVE` with the identifiers from inspection.
For supported text fields, compose a replacement in a file instead:

```bash
card resolve CONFLICT --file replacement.md
```

New Summary text follows the board's current Summary limit.
Selecting a retained alternative does not reapply that limit.
The conflict identifier records which alternatives you reviewed.
If they change before resolution, refresh the inspection and review the new set.
Resolution retains history; later concurrent changes can introduce another conflict.

## Repair related values together

Some conflicts involve relationships or an incompatible combination of
kind, lifecycle, and waiting state.
Inspection identifies the contributing assignments.
Use the corresponding repair options instead of choosing one scalar alternative:

| Repair | Options for `card resolve CONFLICT` |
| --- | --- |
| Discard a relationship assignment | `--remove-assignment ALTERNATIVE` |
| Choose a containment parent | `--parent %cm-child:%cm-parent` |
| Remove a containment parent | `--unparent %cm-child` |
| Recheck retained relationships after another edit | `--relationships` |
| Clear waiting | `--waiting clear` |
| Keep waiting | `--waiting keep` |
| Select one of several waiting values | `--waiting-assignment ALTERNATIVE` |
| Choose a kind or lifecycle with a waiting repair | `--kind` or `--lifecycle` |

Repeat relationship options to repair a whole affected component.
The resulting graph must satisfy containment and dependency rules.
Execution repairs must select compatible values;
Cardamom does not invent a default kind or lifecycle.
An unsuccessful repair leaves the conflict and current values unchanged.

The web UI's **Conflicts** tab provides the same inspection and repair operations.
Aggregate views allow inspection; resolution requires a writable board view.

Clone preserves conflicts, alternative identities, and their retained history.
The current portable backup format rejects boards with unresolved conflicts
or ambiguous reference markers because it cannot preserve that state.
