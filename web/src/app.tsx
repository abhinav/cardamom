import { useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Settings } from "lucide-react";
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

import type { AttachmentClient, ChangeClient, WebClient } from "./api.ts";
import { ApprovalsRoute } from "./approvals/approvals.tsx";
import {
  BoardSettingsDialog,
} from "./board-settings.tsx";
import { BoardPickerRoute, BoardSelector } from "./board-selector.tsx";
import {
  collectionRouteSearch,
  issueFiltersFromSearch,
  issueViewFromSearch,
  routineRetiredFromSearch,
  routineRetiredSearch,
  type IssueCollectionMode,
  type IssueFilterNavigation,
} from "./collection-route.ts";
import { ConfigurationRoute } from "./configuration.tsx";
import {
  boardScopePath,
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
import { DocumentTitle } from "./document-title.tsx";
import { type StreamStatus, watchContinuously } from "./invalidation.ts";
import { bootstrapQueryOptions } from "./query-runtime.ts";
import { BoardRoute, ListRoute } from "./issue-views.tsx";
import { IssueDetailPage } from "./issue-detail/issue-detail.tsx";
import {
  defaultBoardView,
  defaultListView,
  listViewForLabel,
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
import { RoutinesRoute } from "./routines/routines.tsx";
import {
  effectiveMutationCapability,
  ServerAccessProvider,
  useServerAccess,
} from "./server-access.tsx";

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
        <button type="button" onClick={() => void bootstrap.refetch()}>
          Retry
        </button>
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
  const pathname = useLocation().pathname;
  const selection = routeBoardScope(pathname);
  const selectionKey = scopeKey(selection);
  const scope = useMemo(
    () => toBoardScopeMessage(selection),
    [selectionKey],
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
        boards={bootstrap.boards}
        preferences={preferences}
        projects={bootstrap.projects}
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
  boards: readonly BoardSummary[];
  preferences: Preferences;
  projects: readonly Project[];
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  streamStatus: StreamStatus;
  updatePreferences: (preferences: Preferences) => void;
  version: string;
}

function ApplicationShell({
  attachmentClient,
  boards,
  preferences,
  projects,
  scope,
  selection,
  streamStatus,
  updatePreferences,
  version,
}: ApplicationShellProps) {
  const { canMutateServer } = useServerAccess();
  const navigate = useNavigate();
  const location = useLocation();
  const collectionRoute = isCollectionRoute(location.pathname);
  const [boardSettingsOpen, setBoardSettingsOpen] = useState(false);
  const selectionIdentity = scopeKey(selection);
  useEffect(() => setBoardSettingsOpen(false), [selectionIdentity]);
  const selectedBoard =
    selection.kind === "board"
      ? boards.find((board) => board.id === selection.boardId)
      : undefined;
  const boardName =
    selection.kind === "unresolved"
      ? "Boards"
      : selection.kind === "all"
        ? "All boards"
        : (selectedBoard?.name ?? `${selection.boardId} unavailable`);
  const selectLabel = (label: string) => {
    updatePreferences({
      ...preferences,
      listView: listViewForLabel(preferences.listView, label),
    });
    if (selection.kind !== "unresolved") {
      navigate(boardScopePath(selection, "list"));
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
    if (selection.kind === "unresolved") {
      return "/";
    }
    const path = boardScopePath(selection, mode);
    return activeCollectionFilters === undefined
      ? path
      : path + collectionRouteSearch(activeCollectionFilters, mode);
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
            boards={boards}
            projects={projects}
            selection={selection}
            onOpenBoardSettings={
              canMutateServer && selectedBoard !== undefined
                ? () => setBoardSettingsOpen(true)
                : undefined
            }
            onSelectScope={(nextSelection) =>
              navigate(
                boardScopePath(
                  nextSelection,
                  routeBoardPage(location.pathname),
                ),
              )
            }
          />
          <StreamState status={streamStatus} />
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
          {selection.kind !== "unresolved" && (
            <>
              <NavLink to={boardScopePath(selection, "approvals")} end>
                Approvals
              </NavLink>
              <NavLink to={collectionPath("list")} end>
                List
              </NavLink>
              <NavLink to={boardScopePath(selection, "routines")} end>
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
            canMutateServer={canMutateServer}
            preferences={preferences}
            projects={projects}
            scope={scope}
            selection={selection}
            selectLabel={selectLabel}
            updatePreferences={updatePreferences}
          />
        </main>
        {canMutateServer && boardSettingsOpen && selectedBoard !== undefined && (
          <BoardSettingsDialog
            key={selectedBoard.id}
            actor={preferences.actor}
            boardId={selectedBoard.id}
            onDismiss={() => setBoardSettingsOpen(false)}
            onSaved={() => setBoardSettingsOpen(false)}
          />
        )}
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
  return (
    <details className="settings-control">
      <summary className="icon-control" aria-label="Settings" title="Settings">
        <Settings aria-hidden="true" />
      </summary>
      <div className="settings-panel">
        <div className="session-controls">
          <label>
            <span>Actor</span>
            <input
              type="text"
              value={preferences.actor}
              placeholder="Not set"
              autoComplete="username"
              onInput={(event) =>
                updatePreferences({
                  ...preferences,
                  actor: event.currentTarget.value,
                })
              }
            />
          </label>
          <label>
            <span>Theme</span>
            <select
              value={preferences.theme}
              onChange={(event) =>
                updatePreferences({
                  ...preferences,
                  theme: event.currentTarget.value as ThemePreference,
                })
              }
            >
              <option value="dark">Dark</option>
              <option value="light">Light</option>
            </select>
          </label>
          <label className="session-toggle">
            <input
              type="checkbox"
              checked={preferences.boardView.showEmptyColumns}
              onChange={(event) =>
                updatePreferences({
                  ...preferences,
                  boardView: {
                    ...preferences.boardView,
                    showEmptyColumns: event.currentTarget.checked,
                  },
                })
              }
            />
            <span>Show empty columns</span>
          </label>
          {selectedBoard !== undefined && (
            <button
              type="button"
              className="session-action"
              onClick={(event) => {
                event.currentTarget.closest("details")?.removeAttribute("open");
                openConfiguration();
              }}
            >
              Configuration
            </button>
          )}
        </div>
        <p className="settings-version">Cardamom version {version}</p>
      </div>
    </details>
  );
}

function StreamState({ status }: { status: StreamStatus }) {
  const label = status[0]?.toUpperCase() + status.slice(1);
  return (
    <span className="stream-state" data-status={status} role="status" aria-live="polite">
      <span className="stream-state-dot" aria-hidden="true" />
      {label}
    </span>
  );
}

function RouteContent({
  attachmentClient,
  boards,
  canMutateServer,
  preferences,
  projects,
  scope,
  selection,
  selectLabel,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
  canMutateServer: boolean;
  preferences: Preferences;
  projects: readonly Project[];
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
        search: collectionRouteSearch(filters, mode),
      },
      { replace: navigation === "replace" },
    );
  const boardRoute = (
    <BoardRoute
      actor={preferences.actor}
      attachmentClient={attachmentClient}
      boards={boards}
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
      boards={boards}
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
    <Routes>
      <Route
        path="/"
        element={<BoardPickerRoute boards={boards} projects={projects} />}
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
        }
      />
      <Route
        path="/board/:boardId/issue/:issueId"
        element={
          <IssuePage
            attachmentClient={attachmentClient}
            preferences={preferences}
            selectLabel={selectLabel}
            updatePreferences={updatePreferences}
          />
        }
      />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
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
  preferences,
  selectLabel,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  preferences: Preferences;
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
      collapsedDetailsBoardIds={preferences.collapsedIssueDetailsBoardIds}
      expectedBoardId={boardId}
      issueId={issueId}
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
}
