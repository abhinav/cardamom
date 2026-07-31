import { useTransport } from "@connectrpc/connect-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Settings } from "lucide-react";
import {
  Link,
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
import { BoardSelector } from "./board-selector.tsx";
import { ConfigurationRoute } from "./configuration.tsx";
import {
  resolveBoardScope,
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
  listViewForLabel,
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
  const selection = resolveBoardScope(
    preferences.boardScope,
    bootstrap.boards,
    bootstrap.serverDefaultBoardId,
  );
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
        storage={storage}
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
  storage: PreferencesStorage;
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
  storage,
  streamStatus,
  updatePreferences,
  version,
}: ApplicationShellProps) {
  const { canMutateServer } = useServerAccess();
  const navigate = useNavigate();
  const collectionRoute = isCollectionRoute(useLocation().pathname);
  const [boardSettingsBoardId, setBoardSettingsBoardId] = useState<
    string | undefined
  >();
  const selectedBoard =
    selection.kind === "board"
      ? boards.find((board) => board.id === selection.boardId)
      : undefined;
  const boardName =
    selection.kind === "unresolved"
      ? "Select a board"
      : selection.kind === "all"
        ? "All boards"
        : (selectedBoard?.name ?? `${selection.boardId} unavailable`);
  const selectLabel = (label: string) => {
    updatePreferences({
      ...preferences,
      listView: listViewForLabel(preferences.listView, label),
    });
    navigate("/list");
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
              canMutateServer ? setBoardSettingsBoardId : undefined
            }
            onSelectScope={(boardScope) =>
              updatePreferences({ ...preferences, boardScope })
            }
          />
          <StreamState status={streamStatus} />
          <SettingsControl
            preferences={preferences}
            selectedBoard={selectedBoard}
            openConfiguration={() => navigate("/configuration")}
            updatePreferences={updatePreferences}
            version={version}
          />
        </header>

        <nav className="primary-nav" aria-label="Primary">
          <NavLink to="/" end>
            Board
          </NavLink>
          <NavLink to="/approvals" end>
            Approvals
          </NavLink>
          <NavLink to="/list" end>
            List
          </NavLink>
          <NavLink to="/routines" end>
            Routines
          </NavLink>
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
            scope={scope}
            selection={selection}
            selectLabel={selectLabel}
            storage={storage}
            updatePreferences={updatePreferences}
          />
        </main>
        {canMutateServer && boardSettingsBoardId !== undefined && (
          <BoardSettingsDialog
            key={boardSettingsBoardId}
            actor={preferences.actor}
            boardId={boardSettingsBoardId}
            onDismiss={() => setBoardSettingsBoardId(undefined)}
            onSaved={() => setBoardSettingsBoardId(undefined)}
          />
        )}
      </div>
    </>
  );
}

export function isCollectionRoute(pathname: string): boolean {
  return pathname === "/" || pathname === "/list";
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
  scope,
  selection,
  selectLabel,
  storage,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
  canMutateServer: boolean;
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
  storage: PreferencesStorage;
  updatePreferences: (preferences: Preferences) => void;
}) {
  return (
    <Routes>
      <Route
        path="/"
        element={
          <BoardRoute
            actor={preferences.actor}
            attachmentClient={attachmentClient}
            boards={boards}
            canMutateServer={canMutateServer}
            selection={selection}
            selectLabel={selectLabel}
            view={preferences.boardView}
            updateView={(boardView) =>
              updatePreferences({ ...preferences, boardView })
            }
          />
        }
      />
      <Route
        path="/approvals"
        element={
          <ApprovalsPage
            canMutateServer={canMutateServer}
            preferences={preferences}
            scope={scope}
            selection={selection}
          />
        }
      />
      <Route
        path="/list"
        element={
          <ListRoute
            actor={preferences.actor}
            attachmentClient={attachmentClient}
            boards={boards}
            canMutateServer={canMutateServer}
            selection={selection}
            selectLabel={selectLabel}
            view={preferences.listView}
            updateView={(listView) =>
              updatePreferences({ ...preferences, listView })
            }
          />
        }
      />
      <Route
        path="/routines"
        element={
          <RoutinesPage
            canMutateServer={canMutateServer}
            preferences={preferences}
            scope={scope}
            selection={selection}
            selectLabel={selectLabel}
            storage={storage}
          />
        }
      />
      <Route
        path="/configuration"
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
        path="/issues/:issueId"
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
  storage,
}: {
  canMutateServer: boolean;
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
  storage: PreferencesStorage;
}) {
  const requestKey = scopeKey(selection);
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
      showScopeMutationNotice={canMutateServer && !scopeAllowsMutations}
      storage={storage}
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
  const { issueId } = useParams<"issueId">();
  if (issueId === undefined) {
    return <NotFoundPage />;
  }
  return (
    <IssueDetailPage
      key={issueId}
      actor={preferences.actor}
      attachmentClient={attachmentClient}
      collapsedDetailsBoardIds={preferences.collapsedIssueDetailsBoardIds}
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
      <Link to="/">Return to Board</Link>
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
