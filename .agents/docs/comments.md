# Comments and documentation

Use this guide when adding, removing, or revising code comments,
symbol documentation, field comments, or package documentation.

## Choose the reader

Documentation is for callers, importers, and package users.
It explains what a symbol promises, how to use it,
and which constraints matter at the boundary.

Implementation comments are for maintainers changing the code.
They explain why code has its shape, which invariant must hold,
or what would break if the code were simplified.

Do not make callers read implementation comments to learn a public contract.
Do not make maintainers infer hidden implementation constraints from public
documentation alone.

## What to document

Every package and exported symbol must have Go documentation.
Document a private named concept when its users need a contract that the name
and type do not carry.

Explain material details such as:

- what the concept represents
- valid values, units, source, ownership, or lifetime
- side effects, ordering, or concurrency requirements
- error meanings
- external systems, protocols, file formats, or domain concepts

Document each struct field separately when its meaning, source, valid values,
or caller obligation is not obvious from its name and type.
Mark directly constructed required fields inline with `// required`.

## Implementation comments

Use comments to reduce context a maintainer must reconstruct.
Explain a non-obvious decision, an invariant, an external contract,
hidden state, or a cohesive phase in a large operation.

Do not keep comments that narrate syntax, repeat a name,
record discarded proposals, or compensate for unclear organization.
If a comment cannot state the relevant contract or invariant clearly,
reconsider the code shape first.

## Formatting

Use `//` comments rather than block comments.
Use full sentences for standalone comments
and sentence fragments for inline comments.
Use semantic line breaks for multi-line comments.
