import { useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  lazy,
  Suspense,
  useEffect,
  useMemo,
  useState,
} from "react";
import { Moon, Settings, Sun } from "lucide-react";
import {
  Link,
  matchPath,
  NavLink,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useNavigationType,
  useParams,
} from "react-router";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { AttachmentClient, ChangeClient, WebClient } from "./api.ts";
import {
  BoardPickerRoute,
  BoardSelector,
  type BoardConfigurationTarget,
} from "./board-selector.tsx";
import {
  collectionRouteLocationSearch,
  issueFiltersFromSearch,
  issueViewFromSearch,
  labelCollectionLocation,
  routineRetiredFromSearch,
  routineRetiredSearch,
  type IssueCollectionMode,
  type IssueFilterNavigation,
} from "./collection-route.ts";
import {
  boardScopePath,
  boardScopeHref,
  boardScopeSearch,
  resolveBoardScopeSelection,
  routeBoardPage,
  routeBoardScope,
  scopeKey,
  toBoardScopeMessage,
  type BoardScopeSelection,
} from "./board-scope.ts";
import type {
  BoardSummary,
  GetBootstrapResponse,
  Project,
} from "./gen/cardamom/private/v1/project_pb.ts";
import type { BoardScope } from "./gen/cardamom/private/v1/scope_pb.ts";
import {
  SourceHealth,
  type AggregateStatus,
  type SourceCatalogEntry,
} from "./gen/cardamom/private/v1/source_pb.ts";
import { DocumentTitle } from "./document-title.tsx";
import { type StreamStatus, watchContinuously } from "./invalidation.ts";
import { bootstrapQueryOptions } from "./query-runtime.ts";
import {
  defaultBoardView,
  defaultListView,
  type IssueFilters,
} from "./issue-collection.ts";
import {
  loadPreferences,
  savePreferences,
  setIssueDetailsCollapsed,
  type Preferences,
  type PreferencesStorage,
  type ThemePreference,
} from "./preferences.ts";
import {
  effectiveMutationCapability,
  ServerAccessProvider,
  useServerAccess,
} from "./server-access.tsx";

const ApprovalsRoute = lazy(() =>
  import("./approvals/approvals.tsx").then(({ ApprovalsRoute }) => ({
    default: ApprovalsRoute,
  }))
);
const BoardRoute = lazy(() =>
  import("./issue-views.tsx").then(({ BoardRoute }) => ({
    default: BoardRoute,
  }))
);
const BoardConfigurationDialog = lazy(() =>
  import("./board-settings.tsx").then(({ BoardConfigurationDialog }) => ({
    default: BoardConfigurationDialog,
  }))
);
const ConfigurationRoute = lazy(() =>
  import("./configuration.tsx").then(({ ConfigurationRoute }) => ({
    default: ConfigurationRoute,
  }))
);
const IssueDetailPage = lazy(() =>
  import("./issue-detail/issue-detail.tsx").then(({ IssueDetailPage }) => ({
    default: IssueDetailPage,
  }))
);
const ListRoute = lazy(() =>
  import("./issue-views.tsx").then(({ ListRoute }) => ({
    default: ListRoute,
  }))
);
const RoutinesRoute = lazy(() =>
  import("./routines/routines.tsx").then(({ RoutinesRoute }) => ({
    default: RoutinesRoute,
  }))
);

interface AppProps {
  client: WebClient;
  storage: PreferencesStorage;
}

export function App({ client, storage }: AppProps) {
  const transport = useTransport();
  const bootstrap = useQuery(bootstrapQueryOptions(transport));

  if (bootstrap.data === undefined) {
    if (bootstrap.error === null) {
      return <StartupState message="Loading Cardamom" />;
    }
    return (
      <StartupState
        message="Cardamom could not connect"
        detail={bootstrap.error.message}
      >
        <Button type="button" onClick={() => void bootstrap.refetch()}>
          Retry
        </Button>
      </StartupState>
    );
  }
  return (
    <LoadedApp
      attachmentClient={client.attachments}
      bootstrap={bootstrap.data}
      changeClient={client.changes}
      storage={storage}
    />
  );
}

interface LoadedAppProps {
  attachmentClient: AttachmentClient;
  bootstrap: GetBootstrapResponse;
  changeClient: ChangeClient;
  storage: PreferencesStorage;
}

function LoadedApp({
  attachmentClient,
  bootstrap,
  changeClient,
  storage,
}: LoadedAppProps) {
  const [preferences, setPreferences] = useState(() => loadPreferences(storage));
  const queryClient = useQueryClient();
  const location = useLocation();
  const pathname = location.pathname;
  const sources = bootstrap.sources ?? [];
  const aggregateMode = bootstrap.aggregateStatus !== undefined;
  const catalog = {
    aggregate: aggregateMode,
    boards: bootstrap.boards,
    projects: bootstrap.projects,
    sources,
  };
  const selection = resolveBoardScopeSelection(
    routeBoardScope(pathname, new URLSearchParams(location.search)),
    catalog,
  );
  const selectionKey = scopeKey(selection);
  const scope = useMemo(
    () => toBoardScopeMessage(selection, catalog),
    [aggregateMode, selectionKey, bootstrap.boards, bootstrap.projects, sources],
  );
  const [streamStatus, setStreamStatus] = useState<StreamStatus>(
    scope === undefined ? "offline" : "connecting",
  );

  useEffect(() => {
    if (scope === undefined) {
      setStreamStatus("offline");
      return;
    }
    const controller = new AbortController();
    void watchContinuously(
      changeClient,
      scope,
      queryClient,
      setStreamStatus,
      controller.signal,
    );
    return () => controller.abort();
  }, [changeClient, queryClient, scope]);

  useEffect(() => applyTheme(preferences.theme), [preferences.theme]);

  const updatePreferences = (next: Preferences) => {
    setPreferences(next);
    savePreferences(storage, next);
  };

  return (
    <ServerAccessProvider accessMode={bootstrap.accessMode}>
      <ApplicationShell
        attachmentClient={attachmentClient}
        aggregateMode={aggregateMode}
        aggregateStatus={bootstrap.aggregateStatus}
        boards={bootstrap.boards}
        preferences={preferences}
        projects={bootstrap.projects}
        sources={sources}
        scope={scope}
        selection={selection}
        streamStatus={streamStatus}
        updatePreferences={updatePreferences}
        version={bootstrap.version}
      />
    </ServerAccessProvider>
  );
}

interface ApplicationShellProps {
  attachmentClient: AttachmentClient;
  aggregateMode: boolean;
  aggregateStatus: AggregateStatus | undefined;
  boards: readonly BoardSummary[];
  preferences: Preferences;
  projects: readonly Project[];
  sources: readonly SourceCatalogEntry[];
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  streamStatus: StreamStatus;
  updatePreferences: (preferences: Preferences) => void;
  version: string;
}

/** boardConfigurationActor enables edits only on the writable local server. */
export function boardConfigurationActor(
  aggregateMode: boolean,
  canMutateServer: boolean,
  actor: string,
): string | undefined {
  return !aggregateMode && canMutateServer ? actor : undefined;
}

function ApplicationShell({
  attachmentClient,
  aggregateMode,
  aggregateStatus,
  boards,
  preferences,
  projects,
  sources,
  scope,
  selection,
  streamStatus,
  updatePreferences,
  version,
}: ApplicationShellProps) {
  const { canMutateServer: serverCanMutate } = useServerAccess();
  const canMutateServer = serverCanMutate && !aggregateMode;
  const navigate = useNavigate();
  const location = useLocation();
  const collectionRoute = isCollectionRoute(location.pathname);
  const [boardConfigurationTarget, setBoardConfigurationTarget] =
    useState<BoardConfigurationTarget>();
  const selectionIdentity = scopeKey(selection);
  useEffect(() => {
    setBoardConfigurationTarget(undefined);
  }, [selectionIdentity]);
  const selectedBoard =
    selection.kind === "board"
      ? boards.find((board) => board.id === selection.boardId)
      : undefined;
  // Archived boards remain explicit read targets, but the shell must not offer
  // mutations that the repository lifecycle guard will reject.
  const canMutateSelection = canMutateServer &&
    (selection.kind !== "board" || selectedBoard?.archived === undefined);
  const boardName =
    selection.kind === "unresolved"
      ? "Boards"
      : selection.kind === "ambiguous"
        ? `${selection.boardId} unavailable`
      : selection.kind === "all"
        ? selection.projectId !== undefined
          ? "Project boards"
          : selection.sourceId !== undefined
            ? "Source boards"
            : "All boards"
        : (selectedBoard?.name ?? `${selection.boardId} unavailable`);
  const selectLabel = (label: string) => {
    const destination = labelCollectionLocation(
      location.pathname,
      location.search,
      label,
    );
    if (destination !== undefined) {
      navigate(destination);
    }
  };
  const activeCollectionMode: IssueCollectionMode =
    routeBoardPage(location.pathname) === "list" ? "list" : "board";
  const activeCollectionFilters = collectionRoute
    ? issueFiltersFromSearch(
        new URLSearchParams(location.search),
        activeCollectionMode,
      )
    : undefined;
  const collectionPath = (mode: IssueCollectionMode): string => {
    if (selection.kind === "unresolved" || selection.kind === "ambiguous") {
      return "/";
    }
    const path = boardScopePath(selection, mode);
    const search = activeCollectionFilters === undefined
      ? boardScopeSearch(selection)
      : collectionRouteLocationSearch(
        boardScopeSearch(selection),
        activeCollectionFilters,
        mode,
      );
    return path + search;
  };

  return (
    <>
      <ScrollToTopAfterPushNavigation />
      <DocumentTitle boardName={boardName} boards={boards} />
      <div
        className={`app-frame${collectionRoute ? " app-frame-collection" : ""}`}
      >
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <header className="app-header">
          <BoardSelector
            aggregate={aggregateMode}
            boards={boards}
            sources={sources}
            projects={projects}
            selection={selection}
            onOpenBoardConfiguration={setBoardConfigurationTarget}
            onSelectScope={(nextSelection) =>
              navigate(
                boardScopeHref(
                  nextSelection,
                  routeBoardPage(location.pathname),
                ),
              )
            }
          />
          <StreamState
            aggregateStatus={aggregateStatus}
            sources={sources}
            status={streamStatus}
          />
          {!aggregateMode && (
            <SettingsControl
              preferences={preferences}
              selectedBoard={selectedBoard}
              openConfiguration={() => {
                if (selection.kind === "board") {
                  navigate(boardScopePath(selection, "settings"));
                }
              }}
              updatePreferences={updatePreferences}
              version={version}
            />
          )}
        </header>

        <nav
          className={
            `primary-nav${
              selection.kind === "unresolved" ? " primary-nav-picker" : ""
            }`
          }
          aria-label="Primary"
        >
          <NavLink
            to={
              selection.kind === "unresolved"
                ? "/"
                : collectionPath("board")
            }
            end
          >
            {selection.kind === "unresolved" ? "Boards" : "Board"}
          </NavLink>
          {selection.kind !== "unresolved" && selection.kind !== "ambiguous" && (
            <>
              <NavLink to={boardScopeHref(selection, "approvals")} end>
                Approvals
              </NavLink>
              <NavLink to={collectionPath("list")} end>
                List
              </NavLink>
              <NavLink to={boardScopeHref(selection, "routines")} end>
                Routines
              </NavLink>
            </>
          )}
        </nav>

        <main
          id="main-content"
          className={
            `main-content${collectionRoute ? " main-content-collection" : ""}`
          }
          tabIndex={-1}
        >
          <RouteContent
            attachmentClient={attachmentClient}
            boards={boards}
            canMutateServer={canMutateSelection}
            aggregateMode={aggregateMode}
            preferences={preferences}
            projects={projects}
            sources={sources}
            scope={scope}
            selection={selection}
            selectLabel={selectLabel}
            updatePreferences={updatePreferences}
          />
        </main>
        <Suspense fallback={null}>
          {boardConfigurationTarget !== undefined && (
            <BoardConfigurationDialog
              key={`${boardConfigurationTarget.source?.sourceId ?? "local"}:${boardConfigurationTarget.id}`}
              actor={boardConfigurationActor(
                aggregateMode,
                serverCanMutate,
                preferences.actor,
              )}
              boardId={boardConfigurationTarget.id}
              source={boardConfigurationTarget.source}
              onDismiss={() => setBoardConfigurationTarget(undefined)}
              onSaved={() => setBoardConfigurationTarget(undefined)}
            />
          )}
        </Suspense>
      </div>
    </>
  );
}

export function isCollectionRoute(pathname: string): boolean {
  return [
    "/all",
    "/all/list",
    "/board/:boardId",
    "/board/:boardId/list",
  ].some((path) => matchPath({ path, end: true }, pathname) !== null);
}

interface SettingsControlProps {
  preferences: Preferences;
  openConfiguration: () => void;
  selectedBoard: BoardSummary | undefined;
  updatePreferences: (preferences: Preferences) => void;
  version: string;
}

export function SettingsControl({
  openConfiguration,
  preferences,
  selectedBoard,
  updatePreferences,
  version,
}: SettingsControlProps) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            aria-label="Settings"
            title="Settings"
            variant="outline"
            size="icon"
          />
        }
      >
        <Settings aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 gap-4 p-4">
        <div className="grid gap-2">
          <Label htmlFor="session-actor">Actor</Label>
          <Input
            id="session-actor"
            type="text"
            value={preferences.actor}
            placeholder="Not set"
            autoComplete="username"
            onChange={(event) =>
              updatePreferences({
                ...preferences,
                actor: event.currentTarget.value,
              })
            }
          />
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm font-medium">Theme</span>
          <Button
            type="button"
            id="session-theme"
            variant="outline"
            size="icon"
            aria-label={
              preferences.theme === "dark"
                ? "Switch to light theme"
                : "Switch to dark theme"
            }
            title={
              preferences.theme === "dark"
                ? "Switch to light theme"
                : "Switch to dark theme"
            }
            onClick={() =>
              updatePreferences({
                ...preferences,
                theme: preferences.theme === "dark" ? "light" : "dark",
              })
            }
          >
            {preferences.theme === "dark"
              ? <Sun aria-hidden="true" />
              : <Moon aria-hidden="true" />}
          </Button>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id="show-empty-columns"
            checked={preferences.boardView.showEmptyColumns}
            onCheckedChange={(checked) =>
              updatePreferences({
                ...preferences,
                boardView: {
                  ...preferences.boardView,
                  showEmptyColumns: checked,
                },
              })
            }
          />
          <Label htmlFor="show-empty-columns">Show empty columns</Label>
        </div>
        {selectedBoard !== undefined && (
          <Button
            type="button"
            className="w-full"
            onClick={() => {
              setOpen(false);
              openConfiguration();
            }}
          >
            Configuration
          </Button>
        )}
        <p className="text-xs text-muted-foreground">
          Cardamom version {version}
        </p>
      </PopoverContent>
    </Popover>
  );
}

function StreamState({
  aggregateStatus,
  sources,
  status,
}: {
  aggregateStatus: AggregateStatus | undefined;
  sources: readonly SourceCatalogEntry[];
  status: StreamStatus;
}) {
  const label = aggregateStatus?.complete === false ? "Degraded" : status[0]?.toUpperCase() + status.slice(1);
  return (
    <Collapsible className="stream-state-details">
      <CollapsibleTrigger
        className="stream-state"
        data-status={status}
        role="status"
        aria-live="polite"
        render={<Button variant="ghost" size="sm" />}
      >
        <span className="stream-state-dot" aria-hidden="true" />
        {label}
      </CollapsibleTrigger>
      {sources.length > 0 && (
        <CollapsibleContent className="source-health-panel">
          <strong>Source health</strong>
          {sources.map((source) => (
            <div key={source.source?.sourceId} className="source-health-row">
              <span>{source.source?.sourceId ?? "Unknown source"}</span>
              <span>{sourceHealthLabel(source.health)}</span>
              {source.diagnostic !== "" && <span>{source.diagnostic}</span>}
            </div>
          ))}
          {aggregateStatus?.complete === false && (
            <p>Some selected sources did not contribute to the current read.</p>
          )}
        </CollapsibleContent>
      )}
    </Collapsible>
  );
}

function sourceHealthLabel(health: SourceHealth): string {
  switch (health) {
    case SourceHealth.HEALTHY:
      return "Healthy";
    case SourceHealth.DEGRADED:
      return "Degraded";
    case SourceHealth.UNAVAILABLE:
      return "Unavailable";
    default:
      return "Unknown";
  }
}

function RouteContent({
  attachmentClient,
  aggregateMode,
  boards,
  canMutateServer,
  preferences,
  projects,
  sources,
  scope,
  selection,
  selectLabel,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  aggregateMode: boolean;
  boards: readonly BoardSummary[];
  canMutateServer: boolean;
  preferences: Preferences;
  projects: readonly Project[];
  sources: readonly SourceCatalogEntry[];
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
  updatePreferences: (preferences: Preferences) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const search = new URLSearchParams(location.search);
  const boardView = issueViewFromSearch(preferences.boardView, search, "board");
  const listView = issueViewFromSearch(preferences.listView, search, "list");
  const updateFilters = (
    filters: IssueFilters,
    mode: IssueCollectionMode,
    navigation: IssueFilterNavigation,
  ) =>
    navigate(
      {
        pathname: location.pathname,
        search: collectionRouteLocationSearch(location.search, filters, mode),
      },
      { replace: navigation === "replace" },
    );
  const boardRoute = (
    <BoardRoute
      actor={preferences.actor}
      attachmentClient={attachmentClient}
      aggregateMode={aggregateMode}
      boards={boards}
      projects={projects}
      sources={sources}
      canMutateServer={canMutateServer}
      selection={selection}
      selectLabel={selectLabel}
      view={boardView}
      updateFilters={(filters, navigation) =>
        updateFilters(filters, "board", navigation)}
      updateView={(boardView) =>
        updatePreferences({
          ...preferences,
          boardView: { ...boardView, filters: defaultBoardView.filters },
        })
      }
    />
  );
  const approvalsRoute = (
    <ApprovalsPage
      canMutateServer={canMutateServer}
      preferences={preferences}
      scope={scope}
      selection={selection}
    />
  );
  const listRoute = (
    <ListRoute
      actor={preferences.actor}
      attachmentClient={attachmentClient}
      aggregateMode={aggregateMode}
      boards={boards}
      projects={projects}
      sources={sources}
      canMutateServer={canMutateServer}
      selection={selection}
      selectLabel={selectLabel}
      view={listView}
      updateFilters={(filters, navigation) =>
        updateFilters(filters, "list", navigation)}
      updateView={(listView) =>
        updatePreferences({
          ...preferences,
          listView: { ...listView, filters: defaultListView.filters },
        })
      }
    />
  );
  const routinesRoute = (
    <RoutinesPage
      canMutateServer={canMutateServer}
      preferences={preferences}
      scope={scope}
      selection={selection}
      selectLabel={selectLabel}
    />
  );
  return (
    <Suspense fallback={<StartupState message="Loading page" />}>
      <Routes>
        <Route
          path="/"
          element={
            <BoardPickerRoute
              aggregate={aggregateMode}
              boards={boards}
              projects={projects}
              sources={sources}
            />
          }
        />
        <Route path="/board/:boardId" element={boardRoute} />
        <Route path="/all" element={boardRoute} />
        <Route path="/board/:boardId/approvals" element={approvalsRoute} />
        <Route path="/all/approvals" element={approvalsRoute} />
        <Route path="/board/:boardId/list" element={listRoute} />
        <Route path="/all/list" element={listRoute} />
        <Route path="/board/:boardId/routines" element={routinesRoute} />
        <Route path="/all/routines" element={routinesRoute} />
        <Route
          path="/board/:boardId/settings"
          element={
            aggregateMode
              ? <NotFoundPage />
              : (
                <ConfigurationRoute
                  actor={preferences.actor}
                  boardId={selection.kind === "board" ? selection.boardId : undefined}
                  boardName={
                    selection.kind === "board"
                      ? boards.find((board) => board.id === selection.boardId)?.name
                      : undefined
                  }
                  canMutateServer={canMutateServer}
                />
              )
          }
        />
        <Route
          path="/board/:boardId/issue/:issueId"
          element={
            <IssuePage
              attachmentClient={attachmentClient}
              boards={boards}
              preferences={preferences}
              projects={projects}
              canMutateServer={canMutateServer}
              selection={selection}
              selectLabel={selectLabel}
              updatePreferences={updatePreferences}
            />
          }
        />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  );
}

function ApprovalsPage({
  canMutateServer,
  preferences,
  scope,
  selection,
}: {
  canMutateServer: boolean;
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
}) {
  const requestKey = scopeKey(selection);
  const scopeAllowsMutations = selection.kind !== "all";
  const canResolveCheckpoints = effectiveMutationCapability(
    canMutateServer,
    scopeAllowsMutations,
  );
  return (
    <ApprovalsRoute
      key={requestKey}
      actor={preferences.actor}
      canResolveCheckpoints={canResolveCheckpoints}
      requestKey={requestKey}
      scope={scope}
      showDecisionControls={canMutateServer}
      showScopeMutationNotice={canMutateServer && !scopeAllowsMutations}
    />
  );
}

function RoutinesPage({
  canMutateServer,
  preferences,
  scope,
  selection,
  selectLabel,
}: {
  canMutateServer: boolean;
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const requestKey = scopeKey(selection);
  const showRetired = routineRetiredFromSearch(
    new URLSearchParams(location.search),
  );
  const scopeAllowsMutations = selection.kind !== "all";
  const canMutateRoutines = effectiveMutationCapability(
    canMutateServer,
    scopeAllowsMutations,
  );
  return (
    <RoutinesRoute
      key={requestKey}
      actor={preferences.actor}
      canMutateRoutines={canMutateRoutines}
      requestKey={requestKey}
      scope={scope}
      selectLabel={selectLabel}
      showRetired={showRetired}
      showScopeMutationNotice={canMutateServer && !scopeAllowsMutations}
      updateShowRetired={(next) =>
        navigate({
          pathname: location.pathname,
          search: routineRetiredSearch(next),
        })}
    />
  );
}

function IssuePage({
  attachmentClient,
  boards,
  preferences,
  projects,
  canMutateServer,
  selection,
  selectLabel,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
  preferences: Preferences;
  projects: readonly Project[];
  canMutateServer: boolean;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
  updatePreferences: (preferences: Preferences) => void;
}) {
  const { boardId, issueId } = useParams<"boardId" | "issueId">();
  if (boardId === undefined || issueId === undefined) {
    return <NotFoundPage />;
  }
  return (
    <IssueDetailPage
      key={`${boardId}:${issueId}`}
      actor={preferences.actor}
      attachmentClient={attachmentClient}
      source={selection.kind === "board" ? selection.source : undefined}
      boards={boards}
      collapsedDetailsBoardIds={preferences.collapsedIssueDetailsBoardIds}
      expectedBoardId={boardId}
      issueId={issueId}
      projects={projects}
      readOnly={!canMutateServer}
      relationsOpen={preferences.relationsOpen}
      selectLabel={selectLabel}
      setRelationsOpen={(relationsOpen) =>
        updatePreferences({ ...preferences, relationsOpen })
      }
      setDetailsCollapsed={(boardId, collapsed) =>
        updatePreferences(
          setIssueDetailsCollapsed(preferences, boardId, collapsed),
        )
      }
    />
  );
}

function NotFoundPage() {
  return (
    <section className="route-placeholder" aria-labelledby="route-title">
      <p className="route-kicker">404</p>
      <h1 id="route-title">Page not found</h1>
      <Link to="/">Return to board picker</Link>
    </section>
  );
}

function ScrollToTopAfterPushNavigation() {
  const location = useLocation();
  const navigationType = useNavigationType();
  useEffect(() => {
    if (navigationType === "PUSH") {
      window.scrollTo({ top: 0 });
    }
  }, [location.key, navigationType]);
  return null;
}

function StartupState({
  children,
  detail,
  message,
}: {
  children?: ReactNode;
  detail?: string;
  message: string;
}) {
  return (
    <main className="startup-state">
      <div className="startup-mark" aria-hidden="true">
        Cardamom
      </div>
      <div role={detail === undefined ? "status" : "alert"}>
        <h1>{message}</h1>
        {detail !== undefined && <p>{detail}</p>}
      </div>
      {children}
    </main>
  );
}

function applyTheme(theme: ThemePreference): void {
  document.documentElement.dataset.theme = theme;
  document.documentElement.classList.toggle("dark", theme === "dark");
}
