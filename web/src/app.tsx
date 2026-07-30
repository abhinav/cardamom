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
  boardSettingsBoardId,
} from "./board-settings.tsx";
import { ConfigurationRoute } from "./configuration.tsx";
import {
  resolveBoardScope,
  scopeKey,
  toBoardScopeMessage,
  type BoardScopePreference,
  type BoardScopeSelection,
} from "./board-scope.ts";
import type {
  BoardSummary,
  GetBootstrapResponse,
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
    <ApplicationShell
      attachmentClient={attachmentClient}
      boards={bootstrap.boards}
      preferences={preferences}
      scope={scope}
      selection={selection}
      storage={storage}
      streamStatus={streamStatus}
      updatePreferences={updatePreferences}
    />
  );
}

interface ApplicationShellProps {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  storage: PreferencesStorage;
  streamStatus: StreamStatus;
  updatePreferences: (preferences: Preferences) => void;
}

function ApplicationShell({
  attachmentClient,
  boards,
  preferences,
  scope,
  selection,
  storage,
  streamStatus,
  updatePreferences,
}: ApplicationShellProps) {
  const navigate = useNavigate();
  const collectionRoute = isCollectionRoute(useLocation().pathname);
  const [boardSettingsOpen, setBoardSettingsOpen] = useState(false);
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
  const settingsBoardId = boardSettingsBoardId(selection);
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
          <div className="brand-block">
            <Link className="brand" to="/">
              Cardamom
            </Link>
            <span className="scope-name">{boardName}</span>
          </div>
          <StreamState status={streamStatus} />
          <SettingsControl
            boards={boards}
            preferences={preferences}
            selection={selection}
            selectedBoard={selectedBoard}
            openBoardSettings={() => setBoardSettingsOpen(true)}
            openConfiguration={() => navigate("/configuration")}
            updatePreferences={updatePreferences}
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
            preferences={preferences}
            scope={scope}
            selection={selection}
            selectLabel={selectLabel}
            storage={storage}
            updatePreferences={updatePreferences}
          />
        </main>
        {boardSettingsOpen && settingsBoardId !== undefined && (
          <BoardSettingsDialog
            key={settingsBoardId}
            actor={preferences.actor}
            boardId={settingsBoardId}
            onDismiss={() => setBoardSettingsOpen(false)}
            onSaved={() => setBoardSettingsOpen(false)}
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
  boards: readonly BoardSummary[];
  preferences: Preferences;
  openBoardSettings: () => void;
  openConfiguration: () => void;
  selection: BoardScopeSelection;
  selectedBoard: BoardSummary | undefined;
  updatePreferences: (preferences: Preferences) => void;
}

export function SettingsControl({
  boards,
  openBoardSettings,
  openConfiguration,
  preferences,
  selection,
  selectedBoard,
  updatePreferences,
}: SettingsControlProps) {
  const unavailableSelection =
    selection.kind === "board" && selectedBoard === undefined;
  return (
    <details className="settings-control">
      <summary className="icon-control" aria-label="Settings" title="Settings">
        <Settings aria-hidden="true" />
      </summary>
      <div className="settings-panel">
        <div className="session-controls">
          <label className="session-board-control">
            <span>Board</span>
            <select
              value={scopeKey(selection)}
              onChange={(event) => {
                const value = event.currentTarget.value;
                const boardScope: BoardScopePreference =
                  value === "all"
                    ? { kind: "all" }
                    : { kind: "board", boardId: value.slice("board:".length) };
                updatePreferences({ ...preferences, boardScope });
              }}
            >
              {selection.kind === "unresolved" && (
                <option value="unresolved" disabled>
                  Select a board
                </option>
              )}
              {unavailableSelection && (
                <option value={scopeKey(selection)} disabled>
                  {selection.boardId} (unavailable)
                </option>
              )}
              <option value="all">All boards</option>
              {boards.map((board) => (
                <option key={board.id} value={`board:${board.id}`}>
                  {board.name}
                </option>
              ))}
            </select>
          </label>
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
            <>
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
              <button
                type="button"
                className="session-action"
                onClick={(event) => {
                  event.currentTarget.closest("details")?.removeAttribute("open");
                  openBoardSettings();
                }}
              >
                Board settings
              </button>
            </>
          )}
        </div>
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
  preferences,
  scope,
  selection,
  selectLabel,
  storage,
  updatePreferences,
}: {
  attachmentClient: AttachmentClient;
  boards: readonly BoardSummary[];
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
  preferences,
  scope,
  selection,
}: {
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
}) {
  const requestKey = scopeKey(selection);
  return (
    <ApprovalsRoute
      key={requestKey}
      actor={preferences.actor}
      readOnly={selection.kind === "all"}
      requestKey={requestKey}
      scope={scope}
    />
  );
}

function RoutinesPage({
  preferences,
  scope,
  selection,
  selectLabel,
  storage,
}: {
  preferences: Preferences;
  scope: BoardScope | undefined;
  selection: BoardScopeSelection;
  selectLabel: (label: string) => void;
  storage: PreferencesStorage;
}) {
  const requestKey = scopeKey(selection);
  return (
    <RoutinesRoute
      key={requestKey}
      actor={preferences.actor}
      readOnly={selection.kind === "all"}
      requestKey={requestKey}
      scope={scope}
      selectLabel={selectLabel}
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
