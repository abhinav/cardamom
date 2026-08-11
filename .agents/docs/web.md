# Web visual language

Use this guide when changing browser layout, components, or styles under
`web/`.

## Semantic presentation

Represent the same product concept with the same visible label, semantic token,
and compact visual form across routes.
Use `IssueStatusBadge` when issue status must stand alone
and `IssueStatusDot` when a nearby graph row supplies the issue title.
Board columns may use the shared issue-state color as a rail
because the column heading already names the state.

Color reinforces meaning;
visible text or surrounding structure carries the meaning without color.
Use the role-based colors defined in `app.css`.
Do not put raw state or feedback colors in route styles.

Use `state-badge` for compact operational state
and `metadata-chip` for labels and classification metadata.
Do not make metadata look like an actionable control.

## Surfaces and shape

Keep in-flow cards, tables, kanban columns, and content panels flat.
Separate ordinary content with spacing, opaque surface colors,
and one-pixel borders.
Reserve `--shadow-overlay` for menus, dialogs,
and other content that visibly covers the current page.

Use the shared radius tokens by role:

- `--radius-detail` for compact badges and labels.
- `--radius-control` for controls and ordinary containers.
- `--radius-overlay` for dialogs and floating panels.
- `--radius-pill` only for compact state or count capsules.

Keep primary application and data surfaces opaque.
Transparency is appropriate for modal scrims
and transient interaction effects whose composited contrast remains readable.

## Loading boundaries

Treat major routes as browser loading boundaries.
Keep application-shell dependencies limited to code needed
before a route is selected,
and load page-owned code through route-level dynamic imports.

Investigate production bundle-size warnings as dependency-boundary signals.
Do not suppress a warning by raising its threshold
unless the larger chunk is deliberate and its loading cost has been measured.

After changing a loading boundary,
exercise every affected route in the in-app Browser
and check the browser console for loading failures.

## Interaction and validation

Primary actions use the accent button treatment.
Destructive secondary choices use `danger-button` consistently.
Do not rely on color alone to distinguish either action.

Preserve a visible keyboard focus indicator on every interactive element.
After a visual change, inspect each affected route in the in-app Browser
at desktop and mobile widths in both light and dark themes.
Check content density, text wrapping, horizontal overflow, focus visibility,
and semantic consistency with neighboring routes.
