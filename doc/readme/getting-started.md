# Getting started

Cardamom task-management commands are primarily for agents.
This example shows a hypothetical coordinator and two workers
using them in parallel.

- The coordinator initializes Cardamom and creates two independent tasks.

    ```bash
    export CARDAMOM_ACTOR=coordinator
    card init

    workstream=$(card create --type workstream \
      --summary "Publish the operator and reviewer procedures." \
      "Publish release documentation")

    card create \
      --parent "$workstream" --label area:installation \
      --summary "Document the supported installation paths." \
      "Write the installation guide"

    card create \
      --parent "$workstream" --label area:release \
      --summary "Record each release step and owner." \
      "Write the release guide"
    ```

    The workstream groups the two related tasks.

- Parallel workers receive the tasks,
  using `--watch` to wait for available work.

    ```bash
    card --actor worker1 --json claim --context --watch
    card --actor worker2 --json claim --context --watch
    ```

- Each worker records progress in the task's State.
  This can be used later to resume work if the worker is lost.

    ```bash
    export CARDAMOM_ACTOR=worker1
    card state set <task-id> \
      "The release inputs are identified." \
      --next "Add checksum publication."
    card state commit <task-id>
    ```

- The worker records the task's result and releases it to the coordinator.
  The coordinator accepts and closes that task.

    ```bash
    export CARDAMOM_ACTOR=worker1
    card state set <task-id> \
      "The release guide is ready for acceptance." \
      --next "The coordinator reviews the result."
    card result set <task-id> \
      "Documented the release steps and checksum publication."
    card release <task-id> \
      --waiting "coordinator acceptance"

    export CARDAMOM_ACTOR=coordinator
    card result show <task-id>
    card close <task-id>
    ```

While the agents work,
you can browse live status in the web UI with `card web`,
or add new tasks manually with `card create` when you want to steer the work.
