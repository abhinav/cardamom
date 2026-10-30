import type { MessageInitShape } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import {
  createInfiniteQueryOptions,
  useMutation,
  useTransport,
} from "@connectrpc/connect-query";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  ConflictService,
  type ConflictAlternative,
  type ConflictInspection,
  type ResolveConflictRequestSchema,
} from "../gen/cardamom/private/v1/conflicts_pb.ts";
import type { SourceRef } from "../gen/cardamom/private/v1/source_pb.ts";
import { WatchResource } from "../gen/cardamom/private/v1/change_pb.ts";
import {
  runInvalidatingMutation,
  unaryRouteQueryOptions,
} from "../query-runtime.ts";
import {
  ConflictIssueLinks,
  ConflictValueContent,
  conflictUnitLabel,
} from "./conflict-values.tsx";
import { ExecutionRepairForm, RelationsRepairForm } from "./repairs.tsx";
import "./conflicts.css";

/** ResolutionChoice has no default; every mutation submits a reviewed choice. */
export type ResolutionChoice = NonNullable<
  MessageInitShape<typeof ResolveConflictRequestSchema>["choice"]
>;
type Scope = MessageInitShape<
  typeof import("../gen/cardamom/private/v1/conflicts_pb.ts").ConflictScopeSchema
>;

/** ConflictsRoute reads one board's metadata a page at a time. */
export function ConflictsRoute({
  boardId,
  source,
  canMutate,
}: {
  boardId: string;
  source?: SourceRef;
  canMutate: boolean;
}) {
  const transport = useTransport();
  const [search, setSearch] = useSearchParams();
  const scope = { boardId, source };
  const issueId = search.get("issue") ?? undefined;
  const selected = search.get("conflict");
  const query = useInfiniteQuery({
    ...createInfiniteQueryOptions(
      ConflictService.method.listConflicts,
      { scope, issueId, after: "", limit: 30 },
      {
        transport,
        pageParamKey: "after",
        getNextPageParam: (page) => page.next || undefined,
      },
    ),
    retry: false,
  });
  const select = (id?: string) => {
    const next = new URLSearchParams(search);
    if (id === undefined) next.delete("conflict");
    else next.set("conflict", id);
    setSearch(next);
  };
  return (
    <section className="conflicts-page" aria-labelledby="conflicts-title">
      <header>
        <p className="route-kicker">Board</p>
        <h1 id="conflicts-title">Conflicts</h1>
        <p>
          Review competing edits, then choose a value or repair the affected
          relationships.
        </p>
        {issueId && (
          <p>
            Showing conflicts for {issueId}.{" "}
            <Button
              variant="link"
              onClick={() => setSearch(new URLSearchParams())}
            >
              Show all board conflicts
            </Button>
          </p>
        )}
        {!canMutate && (
          <p>
            This server permits inspection only. Resolve conflicts on a writable
            source.
          </p>
        )}
      </header>
      {selected === null ? (
        <>
          {query.isPending && <p role="status">Loading conflicts…</p>}
          {query.error && (
            <p role="alert">
              {query.error.message}{" "}
              <Button onClick={() => void query.refetch()}>Retry</Button>
            </p>
          )}
          {query.data?.pages
            .flatMap((page) => page.conflicts)
            .map((conflict) => (
              <article className="conflict-row" key={conflict.id}>
                <div>
                  <h2>
                    {conflict.constraint
                      ? `${conflict.constraint === "execution" ? "Execution" : "Relationship"} constraint`
                      : conflictUnitLabel(conflict.unit)}
                  </h2>
                  <ConflictIssueLinks issues={conflict.issues} />
                  <p>
                    {conflict.alternatives}{" "}
                    {conflict.constraint
                      ? "contributing assignments"
                      : "competing values"}
                  </p>
                </div>
                <Button variant="outline" onClick={() => select(conflict.id)}>
                  Inspect
                </Button>
              </article>
            ))}
          {query.data?.pages.every((page) => page.conflicts.length === 0) && (
            <p>No unresolved conflicts.</p>
          )}
          {query.hasNextPage && (
            <Button
              disabled={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              Load more conflicts
            </Button>
          )}
        </>
      ) : (
        <ConflictDetail
          key={selected}
          id={selected}
          scope={scope}
          canMutate={canMutate}
          returnToList={() => {
            select();
            void query.refetch();
          }}
        />
      )}
    </section>
  );
}

function ConflictDetail({
  id,
  scope,
  canMutate,
  returnToList,
}: {
  id: string;
  scope: Scope;
  canMutate: boolean;
  returnToList: () => void;
}) {
  const transport = useTransport();
  const queryClient = useQueryClient();
  const query = useQuery(
    unaryRouteQueryOptions(
      ConflictService.method.getConflict,
      { scope, conflictId: id },
      transport,
    ),
  );
  const mutation = useMutation(ConflictService.method.resolveConflict);
  const [failure, setFailure] = useState<ConnectError>();
  const [compose, setCompose] = useState(false);
  const [text, setText] = useState("");
  const inspection = query.data?.conflict;
  const submit = async (choice: ResolutionChoice) => {
    setFailure(undefined);
    try {
      await runInvalidatingMutation(
        queryClient,
        [
          WatchResource.BOARD,
          WatchResource.BOARD_CATALOG,
          WatchResource.ISSUES,
        ],
        () => mutation.mutateAsync({ scope, conflictId: id, choice }),
      );
      returnToList();
    } catch (error) {
      setFailure(ConnectError.from(error));
    }
  };
  const stale = isStaleReview(query.error) || isStaleReview(failure);
  const disabled = mutation.isPending || stale;
  return (
    <div className="conflict-detail">
      <Button variant="outline" onClick={returnToList}>
        Back to conflicts
      </Button>
      {query.isPending && <p role="status">Loading alternatives…</p>}
      {stale ? (
        <div role="alert">
          <p>
            This conflict changed or was resolved. Refresh the list before
            reviewing it again.
          </p>
          <Button onClick={returnToList}>Refresh conflict list</Button>
        </div>
      ) : query.error ? (
        <div role="alert">
          <p>{query.error.message}</p>
          <Button onClick={() => void query.refetch()}>Retry inspection</Button>
        </div>
      ) : null}
      {inspection && (
        <>
          <h2>
            {inspection.summary?.constraint
              ? "Repair incompatible assignments"
              : conflictUnitLabel(inspection.summary?.unit)}
          </h2>
          <ConflictIssueLinks issues={inspection.summary?.issues ?? []} />
          {failure && !stale && (
            <div role="alert">
              <p>{failure.rawMessage}</p>
            </div>
          )}
          {!stale && (
            <div className="conflict-alternatives">
              {inspection.alternatives.map((alternative, index) => (
                <Alternative
                  key={alternative.id}
                  alternative={alternative}
                  index={index}
                  scope={scope}
                  conflictId={id}
                  canTake={canMutate && !inspection.summary?.constraint}
                  disabled={disabled}
                  onStale={setFailure}
                  take={() =>
                    void submit({ case: "take", value: alternative.id })
                  }
                />
              ))}
            </div>
          )}
          {canMutate && inspection.summary?.constraint === "relationships" && (
            <RelationsRepairForm
              inspection={inspection}
              disabled={disabled}
              submit={(value) => void submit({ case: "relations", value })}
            />
          )}
          {canMutate && inspection.summary?.constraint === "execution" && (
            <ExecutionRepairForm
              inspection={inspection}
              disabled={disabled}
              submit={(value) => void submit({ case: "execution", value })}
            />
          )}
          {canMutate && acceptsText(inspection) && (
            <div className="conflict-compose">
              <Button
                variant="outline"
                disabled={disabled}
                onClick={() => setCompose(!compose)}
              >
                Compose replacement text
              </Button>
              {compose && (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    void submit({ case: "text", value: text });
                  }}
                >
                  <Label htmlFor="conflict-replacement">
                    Complete replacement text
                  </Label>
                  <Textarea
                    id="conflict-replacement"
                    value={text}
                    onChange={(event) => setText(event.target.value)}
                    disabled={mutation.isPending}
                    readOnly={stale}
                  />
                  <Button type="submit" disabled={disabled}>
                    Use replacement text
                  </Button>
                </form>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function Alternative({
  alternative,
  index,
  scope,
  conflictId,
  canTake,
  disabled,
  onStale,
  take,
}: {
  alternative: ConflictAlternative;
  index: number;
  scope: Scope;
  conflictId: string;
  canTake: boolean;
  disabled: boolean;
  onStale: (error: ConnectError) => void;
  take: () => void;
}) {
  const transport = useTransport();
  const [expanded, setExpanded] = useState(false);
  const query = useQuery({
    ...unaryRouteQueryOptions(
      ConflictService.method.getConflictAlternative,
      { scope, conflictId, alternativeId: alternative.id },
      transport,
    ),
    enabled: expanded,
  });
  // A body read can discover that the entire reviewed contender set expired.
  // Move that failure to the review so every choice closes together.
  useEffect(() => {
    if (isStaleReview(query.error)) onStale(ConnectError.from(query.error));
  }, [query.error, onStale]);
  return (
    <article className="conflict-alternative">
      <h3>
        Value {index + 1}: {conflictUnitLabel(alternative.unit)}
      </h3>
      <details>
        <summary>
          Originating assignments ({alternative.origins.length})
        </summary>
        <ul>
          {alternative.origins.map((origin) => (
            <li key={`${origin.writer}:${origin.sequence}:${origin.ordinal}`}>
              <code>
                {origin.writer}:{origin.sequence.toString()}:
                {origin.ordinal.toString()}
              </code>
            </li>
          ))}
        </ul>
      </details>
      {!expanded ? (
        <Button variant="outline" onClick={() => setExpanded(true)}>
          Read value {index + 1}
        </Button>
      ) : (
        <>
          {query.isPending && <p role="status">Loading value…</p>}
          {query.error && !isStaleReview(query.error) && (
            <p role="alert">
              {query.error.message}{" "}
              <Button variant="outline" onClick={() => void query.refetch()}>
                Retry value
              </Button>
            </p>
          )}
          {query.data?.value && (
            <>
              <ConflictValueContent value={query.data.value} />
              {canTake && (
                <Button disabled={disabled} onClick={take}>
                  Use value {index + 1}
                </Button>
              )}
            </>
          )}
        </>
      )}
    </article>
  );
}

function isStaleReview(error: unknown): boolean {
  if (!error) return false;
  const code = ConnectError.from(error).code;
  return code === Code.FailedPrecondition || code === Code.NotFound;
}

function acceptsText(value: ConflictInspection): boolean {
  return (
    !value.summary?.constraint &&
    [
      "issue.title.set",
      "issue.summary.set",
      "issue.details.set",
      "issue.result",
      "board.name.set",
      "board.description.set",
    ].includes(value.summary?.unit?.kind ?? "")
  );
}
