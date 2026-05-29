#!/usr/bin/env node
//
// linear-todo.js — turn a Linear issue export into a clu batch graph.
//
// Reads Linear issues as JSON on stdin and emits one clu batch document.
// Each issue carries a stable `key` (linear:<id>) so re-running the import
// is idempotent — clu skips (or with `--on-existing update` re-syncs) issues
// whose key already exists, instead of duplicating them.
//
// USAGE
//   linear issue query --all-teams --state unstarted --json --limit 0 --no-pager \
//     | node linear-todo.js \
//     | clu batch --group "Linear Todo import"
//
//   # idempotent re-run; sync titles/priority from Linear:
//   linear issue query ... --json | node linear-todo.js \
//     | clu batch --group "Linear Todo import" --on-existing update
//
// FIELD MAPPING — adjust to your `linear` CLI's JSON shape. This reads the
// common fields defensively and falls back where it can; if your CLI names
// fields differently, tweak pick()/the mapping below.

import { Graph, usage } from "./clu.js";

// Read all of stdin.
let raw = "";
process.stdin.setEncoding("utf8");
for await (const chunk of process.stdin) raw += chunk;
raw = raw.trim();
if (!raw) usage("linear-todo.js: no JSON on stdin (pipe `linear issue query --json`)", 2);

let parsed;
try {
  parsed = JSON.parse(raw);
} catch (e) {
  usage(`linear-todo.js: stdin is not valid JSON: ${e.message}`, 2);
}

// Accept an array, {issues:[...]}, or {nodes:[...]} (Linear GraphQL shape).
const items = Array.isArray(parsed)
  ? parsed
  : parsed.issues || parsed.nodes || [];
if (items.length === 0) usage("linear-todo.js: no issues found in input", 0);

const pick = (o, ...keys) => {
  for (const k of keys) if (o[k] != null && o[k] !== "") return o[k];
  return undefined;
};

// Linear priority: 0 none, 1 urgent, 2 high, 3 medium, 4 low.
// clu priority:    0 highest … 4 lowest. Map urgent→0 … low→3, none→2.
const PRIO = { 0: 2, 1: 0, 2: 1, 3: 2, 4: 3 };

const g = new Graph();
for (const it of items) {
  // Stable identity: prefer the immutable UUID, fall back to the human key.
  const ext = pick(it, "id", "identifier", "number");
  const human = pick(it, "identifier", "id");
  if (ext == null) continue; // can't key it → skip rather than risk a dup

  const title = pick(it, "title", "name") || `${human}`;
  const url = pick(it, "url");
  const desc = pick(it, "description", "descriptionMarkdown");
  const lp = pick(it, "priority");
  const fields = {
    key: `linear:${ext}`,
    title: human ? `${human}: ${title}` : title,
    priority: typeof lp === "number" ? PRIO[lp] ?? 2 : 2,
    labels: ["linear"],
  };
  if (desc || url) {
    fields.description = [desc, url && `Linear: ${url}`].filter(Boolean).join("\n\n");
  }
  // Alias: a safe, unique local handle derived from the external id.
  const alias = `lin-${String(ext).replace(/[^A-Za-z0-9._-]/g, "-")}`;
  g.add(alias, fields);
}

g.emit();
