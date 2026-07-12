#!/usr/bin/env node
//
// migrate.js — a PHASED generator built with the clu.js phase() helper.
//
// Mirrors the "across N phases" shape: each phase is a group of per-module
// tasks, and a phase can't start until the previous one is fully done. The
// helper inserts a milestone at each boundary, so phases advance
// automatically (the milestone auto-closes when its phase completes, which
// unblocks the next phase — no human gate; pair with --group for an umbrella
// that self-completes when the whole migration is done).
//
// USAGE
//   node migrate.js <target> <module...> [--approver=NAME]
//
// EXAMPLES
//   node migrate.js solid api ui store | clu batch --group "React→Solid" --dry-run
//   node migrate.js solid api ui store | clu batch --group "React→Solid" --json

import { Graph, parseArgs, usage } from "./clu.js";

const { flags, positional } = parseArgs();
const target = positional[0];
const modules = positional.slice(1);

if (flags.help || !target || modules.length === 0) {
  usage(
    "usage: node migrate.js <target> <module...> [--approver=NAME]\n" +
      "  pipe into:  | clu batch --group \"<name>\" [--dry-run|--json]",
    flags.help ? 0 : 2,
  );
}

const g = new Graph();

g.phase("inventory", () => {
  for (const m of modules) g.add(`inv-${m}`, { title: `Inventory ${m}` });
});

g.phase("analysis", () => {
  for (const m of modules) g.add(`ana-${m}`, { title: `Pattern-analyze ${m}` });
});

g.phase("migrate", () => {
  for (const m of modules) {
    g.add(`mig-${m}`, { title: `Migrate ${m} → ${target}`, priority: 1 });
  }
});

g.phase("verify", () => {
  // An optional human sign-off before the migration is declared done: a
  // checkpoint inside the final phase. Drop --approver for a manual gate.
  g.checkpoint("signoff", {
    title: `Sign off ${target} migration`,
    approvers: flags.approver ? [flags.approver] : undefined,
  });
  g.add("report", { title: `Write ${target} migration report`, priority: 0 });
});

g.emit(); // one JSON document on stdout → | clu batch
