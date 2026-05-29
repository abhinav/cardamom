"use strict";
//
// clu.js — a tiny, zero-dependency helper for building `clu batch` graphs.
//
// NOT the clu SDK, NOT published, NOT a dependency — just a convenience you
// can copy or `require("./clu.js")` from a generator. It only shapes data:
// build a graph, then `emit()` writes one JSON document to stdout for
// `clu batch` to validate and instantiate.
//
// clu batch remains the single source of validation truth — types,
// priorities, cycles, reference resolution. This helper deliberately does
// none of that (so it can't drift from the real validator); the one thing
// it checks locally is alias uniqueness, caught at add() time with a stack
// trace into your code instead of a batch-time error.
//
//   const { Graph } = require("./clu.js");
//   const g = new Graph();
//   const design = g.add("design", { title: "Design", type: "decision" });
//   const impl   = g.add("impl",   { title: "Implement", needs: [design] });
//   g.checkpoint("gate", { title: "Approve", needs: [impl], approvers: ["alice"] });
//   g.add("ship", { title: "Ship", needs: ["gate"] });
//   g.emit();                                   // → pipe into `clu batch`

export class Graph {
  constructor() {
    this._issues = [];
    this._aliases = new Set();
    this._phase = null; // active phase context (set during phase(fn))
    this._phaseCount = 0;
    this._prevGate = null; // previous phase's milestone alias
  }

  // add(alias, fields) appends an issue and RETURNS its alias, so you can
  // pass the return value straight into another issue's `needs` — a typo
  // then becomes a JS reference error, not a clu "unknown ref" at batch
  // time. `fields` is any subset of the issue shape: title, type, priority,
  // assignee, description, notes, capabilities, labels, needs.
  //
  // Inside phase(), each added issue is also stamped with the phase label
  // and wired to wait for the previous phase's gate.
  add(alias, fields = {}) {
    if (typeof alias !== "string" || alias.trim() === "") {
      throw new Error("clu.add: alias must be a non-empty string");
    }
    if (this._aliases.has(alias)) {
      throw new Error(`clu.add: duplicate alias ${JSON.stringify(alias)}`);
    }
    const issue = { alias, ...fields };
    if (this._phase) {
      const label = `phase:${this._phase.index}-${this._phase.name}`;
      issue.labels = [...(issue.labels || []), label];
      if (this._phase.gate) {
        issue.needs = [this._phase.gate, ...(issue.needs || [])];
      }
      this._phase.members.push(alias);
    }
    this._aliases.add(alias);
    this._issues.push(issue);
    return alias;
  }

  // phase(name, fn) groups every issue added inside fn() into an ordered
  // stage. Tasks in a phase can't start until the previous phase is fully
  // done, and a milestone is inserted at each boundary so advancement is
  // automatic — the milestone auto-closes when its phase completes, which
  // unblocks the next phase (no human gate; use checkpoint() for that).
  // Each task gets a phase:<n>-<name> label for grouping/display. Returns
  // the phase's gate (milestone) alias.
  phase(name, fn) {
    this._phaseCount += 1;
    const index = this._phaseCount;
    this._phase = { index, name, gate: this._prevGate, members: [] };
    fn();
    const { members } = this._phase;
    this._phase = null; // close context before adding the gate itself
    if (members.length === 0) return this._prevGate; // empty phase: no-op

    const gate = `phase-${index}`;
    this.add(gate, {
      title: `Phase ${index}: ${name} — complete`,
      type: "milestone",
      needs: members,
      labels: [`phase:${index}-${name}`],
    });
    this._prevGate = gate;
    return gate;
  }

  // checkpoint(alias, fields) is add() for a manual approval gate. Pass
  // `approvers: [...]` for an approval checkpoint, or omit for a manual one
  // (anyone can clear it). Other fields (title, needs, …) are as add().
  checkpoint(alias, fields = {}) {
    const { approvers, ...rest } = fields;
    const cp = approvers && approvers.length ? { approvers } : {};
    return this.add(alias, { ...rest, checkpoint: cp });
  }

  // issues() returns the raw array, e.g. to inspect or post-process before
  // emitting.
  issues() {
    return this._issues;
  }

  // toJSON() lets JSON.stringify(graph) produce the batch document directly.
  toJSON() {
    return this._issues;
  }

  // emit() writes the document `clu batch` consumes (one JSON value) to the
  // given stream (stdout by default).
  emit(stream = process.stdout) {
    stream.write(JSON.stringify(this._issues, null, 2) + "\n");
  }
}

// parseArgs splits process args into { flags, positional }:
//   --key=value   → flags.key = "value"
//   --flag        → flags.flag = true
//   -h | --help   → flags.help = true
//   anything else → positional[]
// A tiny zero-dependency replacement for the hand-rolled loop every
// generator otherwise repeats at the top.
export function parseArgs(argv = process.argv.slice(2)) {
  const flags = {};
  const positional = [];
  for (const a of argv) {
    if (a === "-h" || a === "--help") {
      flags.help = true;
      continue;
    }
    const m = /^--([^=]+)(?:=(.*))?$/.exec(a);
    if (m) flags[m[1]] = m[2] ?? true;
    else positional.push(a);
  }
  return { flags, positional };
}

// usage prints a message to stderr and exits. Pass code 0 for an explicit
// --help, non-zero for a bad invocation.
export function usage(text, code = 2) {
  process.stderr.write(text.endsWith("\n") ? text : text + "\n");
  process.exit(code);
}
