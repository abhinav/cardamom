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

class Graph {
  constructor() {
    this._issues = [];
    this._aliases = new Set();
  }

  // add(alias, fields) appends an issue and RETURNS its alias, so you can
  // pass the return value straight into another issue's `needs` — a typo
  // then becomes a JS reference error, not a clu "unknown ref" at batch
  // time. `fields` is any subset of the issue shape: title, type, priority,
  // assignee, description, notes, capabilities, labels, needs.
  add(alias, fields = {}) {
    if (typeof alias !== "string" || alias.trim() === "") {
      throw new Error("clu.add: alias must be a non-empty string");
    }
    if (this._aliases.has(alias)) {
      throw new Error(`clu.add: duplicate alias ${JSON.stringify(alias)}`);
    }
    this._aliases.add(alias);
    this._issues.push({ alias, ...fields });
    return alias;
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

module.exports = { Graph };
