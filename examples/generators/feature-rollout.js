#!/usr/bin/env node
//
// feature-rollout.js — a reusable, parameterized workflow GENERATOR.
//
// This is the "codemode" alternative to a static `clu run` YAML template:
// instead of string substitution into a fixed shape, you write real code
// that builds the graph — loops, conditionals, computed fan-out — and pipe
// the result into `clu batch`, which validates it (acyclic, all refs
// resolve) and instantiates it atomically.
//
//   clu generation (this file)        →   clu instantiation (clu batch)
//   any language, any logic               validates + writes one graph, one tx
//
// USAGE
//   node feature-rollout.js <feature> <module...> [--approver=NAME]
//
// EXAMPLES
//   # See the assembled graph without touching the DB:
//   node feature-rollout.js auth-v2 api ui payments | clu batch --dry-run
//
//   # Instantiate it; print the alias→id map:
//   node feature-rollout.js auth-v2 api ui payments --approver=alice | clu batch --json
//
//   # Plain run:
//   node feature-rollout.js search-revamp api ui | clu batch
//
// WHAT IT BUILDS (for `<feature> <m1> <m2> ...`)
//   design                                                   ─┐
//   impl-<m>     needs design                  per module     │ fan-out
//   test-<m>     needs impl-<m>                per module     │
//   audit-<m>    needs impl-<m>                ONLY sensitive │
//   docs         needs every impl                            │
//   gate         needs every test+audit+docs   CHECKPOINT  ←──┘ approval gate
//   ship         needs gate
//
// The security audit is added only for modules whose name looks sensitive
// (payments/auth/billing/...) — the kind of conditional logic a static
// template can't express.
//
// (This example keeps things in the default pool — no capability labels —
// so a fresh `clu ready` surfaces the design step right away. Add
// `capabilities: [...]` to an issue to route it to a specific agent lane;
// note that capability-labelled work is hidden from bare `clu ready` and
// only shows for `clu ready -a <agent>` advertising that capability.)

// The graph below is built RAW (plain objects, no Graph helper) so it reads
// as a direct map of the JSON `clu batch` consumes — port it to any
// language. Only the boilerplate arg parsing borrows clu.js helpers.
import { parseArgs, usage } from "./clu.js";

// ---- args -------------------------------------------------------------

const { flags, positional } = parseArgs();

if (flags.help || positional.length < 2) {
  usage(
    "usage: node feature-rollout.js <feature> <module...> [--approver=NAME]\n" +
      "  pipe into:  | clu batch [--dry-run|--json]",
    flags.help ? 0 : 2,
  );
}

const feature = positional[0];
const modules = positional.slice(1);
const approver = flags.approver || ""; // empty → manual checkpoint (anyone)

// Heuristic: which modules warrant a security audit step.
const SENSITIVE = /pay|auth|billing|account|secret|token|crypto/i;

// ---- graph build ------------------------------------------------------

const issues = [];
const add = (issue) => {
  issues.push(issue);
  return issue.alias;
};

// Root: design the feature.
add({
  alias: "design",
  title: `Design: ${feature}`,
  type: "decision",
  priority: 1,
  description: `Design the ${feature} rollout across: ${modules.join(", ")}.`,
});

// Per-module fan-out: implement → test, plus a conditional security audit.
const gateDeps = []; // everything the approval gate must wait for
const implAliases = [];
for (const mod of modules) {
  const impl = add({
    alias: `impl-${mod}`,
    title: `Implement ${feature} — ${mod}`,
    priority: 1,
    needs: ["design"],
  });
  implAliases.push(impl);

  const test = add({
    alias: `test-${mod}`,
    title: `Test ${feature} — ${mod}`,
    needs: [impl],
  });
  gateDeps.push(test);

  if (SENSITIVE.test(mod)) {
    const audit = add({
      alias: `audit-${mod}`,
      title: `Security audit ${feature} — ${mod}`,
      priority: 0,
      needs: [impl],
      description: `${mod} handles sensitive data — audit before ship.`,
    });
    gateDeps.push(audit);
  }
}

// Docs depend on every implementation.
const docs = add({
  alias: "docs",
  title: `Document: ${feature}`,
  needs: [...implAliases],
});
gateDeps.push(docs);

// Approval checkpoint: gates the release until a human signs off. With an
// --approver it's an "approval" checkpoint; without, a "manual" one. This
// is the piece a generated graph shares with `clu run` workflows.
add({
  alias: "gate",
  title: `Approve release: ${feature}`,
  needs: gateDeps,
  checkpoint: approver ? { approvers: [approver] } : {},
});

// Ship, gated on approval.
add({
  alias: "ship",
  title: `Ship: ${feature}`,
  priority: 0,
  needs: ["gate"],
});

// ---- emit -------------------------------------------------------------
// One JSON document on stdout — nothing else, so it pipes straight into
// `clu batch`. (Use {"issues": issues} or the bare array; both are accepted.)
process.stdout.write(JSON.stringify(issues, null, 2) + "\n");
