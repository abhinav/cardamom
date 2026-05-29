#!/usr/bin/env node
//
// release-train.js — example generator built with the ./clu.js helper.
//
// Per-service build → staging deploy, a single approval checkpoint gating
// production, then per-service prod deploy. Shape: fan-out → fan-in gate →
// fan-out. Contrast with feature-rollout.js, which builds the same kind of
// graph by hand (raw JSON, translatable to any language); this one uses the
// Graph helper so `needs` are passed by handle and dup aliases throw early.
//
// USAGE
//   node release-train.js <version> <service...> [--approver=NAME]
//
// EXAMPLES
//   node release-train.js v1.4.0 api worker web --approver=alice | clu batch --dry-run
//   node release-train.js v1.4.0 api worker web --approver=alice | clu batch --json

"use strict";

const { Graph } = require("./clu.js");

// ---- args -------------------------------------------------------------

const flags = {};
const positional = [];
for (const a of process.argv.slice(2)) {
  const m = /^--([^=]+)=(.*)$/.exec(a);
  if (m) flags[m[1]] = m[2];
  else if (a === "-h" || a === "--help") flags.help = "1";
  else positional.push(a);
}
const version = positional[0];
const services = positional.slice(1);

if (flags.help || !version || services.length === 0) {
  process.stderr.write(
    "usage: node release-train.js <version> <service...> [--approver=NAME]\n" +
      "  pipe into:  | clu batch [--dry-run|--json]\n"
  );
  process.exit(flags.help ? 0 : 2);
}

// ---- graph ------------------------------------------------------------

const g = new Graph();

// Fan-out: build, then deploy each service to staging.
const staged = [];
for (const svc of services) {
  const build = g.add(`build-${svc}`, {
    title: `Build ${svc} @ ${version}`,
    priority: 1,
  });
  staged.push(
    g.add(`stage-${svc}`, {
      title: `Deploy ${svc} to staging`,
      needs: [build], // handle from add() — typo-proof
    })
  );
}

// Fan-in: one approval gate over every staging deploy. With --approver it's
// an approval checkpoint; without, a manual one (anyone can clear it).
const gate = g.checkpoint("prod-gate", {
  title: `Approve production release ${version}`,
  needs: staged,
  approvers: flags.approver ? [flags.approver] : undefined,
  description: `Sign off after verifying all ${services.length} service(s) on staging.`,
});

// Fan-out again: production deploy per service, each gated on approval.
for (const svc of services) {
  g.add(`prod-${svc}`, {
    title: `Deploy ${svc} to production`,
    priority: 0,
    needs: [gate],
  });
}

g.emit(); // one JSON document on stdout → | clu batch
