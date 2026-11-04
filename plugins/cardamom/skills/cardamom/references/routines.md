# Continue recurring work

A routine keeps context across runs initiated by an external scheduler or
coordinator.
It is absent from automatic ready and blocked claim pools;
claim the known routine directly when awakened:

```bash
card --actor <actor> --json claim <routine-id> --context
```

Dependencies still apply to a direct claim.
Leave a blocked routine for the wake policy to retry.

Keep the stable purpose and procedure in Summary or Details.
Use State for the last safe cursor, current targets, and unfinished work;
record run outcomes in history when they help later runs.
Advance the cursor only through successfully processed input.
Keep the operation identifiers and receipts needed to reconcile interrupted
effects before repeating them.

For a completed batch with more work remaining:

```bash
card --actor <actor> state set <routine-id> \
  'Processed records 121–123. Record 124 is unfinished; safe cursor is 123.'
card --actor <actor> release <routine-id>
```

Use `release --waiting '<reason>'` when an external event prevents continuation.
The next awakening must satisfy that reason before claiming.

Close or cancel only when the ongoing assignment ends,
preserving its outcome or abandonment reason.
