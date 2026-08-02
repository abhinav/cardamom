import {
  boardScopePath,
  routeBoardScope,
} from "./board-scope.ts";
import { IssueStatus, IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  defaultBoardView,
  defaultListView,
  type IssueFilters,
  type IssueLifecycleFilter,
  type IssueViewPreferences,
} from "./issue-collection.ts";

/** IssueCollectionMode selects the route defaults for one issue collection. */
export type IssueCollectionMode = "board" | "list";
/** IssueFilterNavigation controls how a filter edit changes browser history. */
export type IssueFilterNavigation = "push" | "replace";

const lifecycleValues: readonly IssueLifecycleFilter[] = [
  "current",
  "open",
  "closed",
  "cancelled",
  "all",
];

const statusValues = [
  ["ready", IssueStatus.READY],
  ["blocked", IssueStatus.BLOCKED],
  ["in-progress", IssueStatus.IN_PROGRESS],
  ["waiting", IssueStatus.WAITING],
  ["closed", IssueStatus.CLOSED],
  ["cancelled", IssueStatus.CANCELLED],
] as const;

const typeValues = [
  ["workstream", IssueType.WORKSTREAM],
  ["task", IssueType.TASK],
  ["checkpoint", IssueType.CHECKPOINT],
  ["routine", IssueType.ROUTINE],
] as const;

/** issueFiltersFromSearch resolves the effective filters for one collection route. */
export function issueFiltersFromSearch(
  search: URLSearchParams,
  mode: IssueCollectionMode,
): IssueFilters {
  const defaults = defaultFilters(mode);
  return {
    lifecycle: parseLifecycle(search.get("lifecycle")) ?? defaults.lifecycle,
    status: parseValue(statusValues, search.get("status")) ?? defaults.status,
    type: parseValue(typeValues, search.get("type")) ?? defaults.type,
    actor: search.get("actor")?.trim() ?? defaults.actor,
    label: search.get("label")?.trim() ?? defaults.label,
    query: search.get("title")?.trim() ?? defaults.query,
  };
}

/** issueViewFromSearch combines URL-owned filters with presentation preferences. */
export function issueViewFromSearch<T extends IssueViewPreferences>(
  view: T,
  search: URLSearchParams,
  mode: IssueCollectionMode,
): T {
  return { ...view, filters: issueFiltersFromSearch(search, mode) };
}

/** collectionRouteSearch serializes non-default filters for a target collection. */
export function collectionRouteSearch(
  filters: IssueFilters,
  mode: IssueCollectionMode,
): string {
  const defaults = defaultFilters(mode);
  const search = new URLSearchParams();
  if (filters.lifecycle !== defaults.lifecycle) {
    search.set("lifecycle", filters.lifecycle);
  }
  setMappedValue(search, "status", statusValues, filters.status, defaults.status);
  setMappedValue(search, "type", typeValues, filters.type, defaults.type);
  setTextValue(search, "actor", filters.actor, defaults.actor);
  setTextValue(search, "label", filters.label, defaults.label);
  setTextValue(search, "title", filters.query, defaults.query);
  const encoded = search.toString();
  return encoded === "" ? "" : `?${encoded}`;
}

/** labelCollectionLocation returns the canonical destination for a label. */
export function labelCollectionLocation(
  pathname: string,
  label: string,
): { pathname: string; search: string } | undefined {
  const selection = routeBoardScope(pathname);
  if (selection.kind === "unresolved") {
    return undefined;
  }
  let mode: IssueCollectionMode = "list";
  if (pathname === boardScopePath(selection, "board")) {
    mode = "board";
  }
  const filters = defaultFilters(mode);
  return {
    pathname: boardScopePath(selection, mode),
    search: collectionRouteSearch({ ...filters, label: label.trim() }, mode),
  };
}

/** routineRetiredFromSearch reports whether retired routines are requested. */
export function routineRetiredFromSearch(search: URLSearchParams): boolean {
  return search.get("retired") === "true";
}

/** routineRetiredSearch serializes retired visibility while omitting its default. */
export function routineRetiredSearch(showRetired: boolean): string {
  return showRetired ? "?retired=true" : "";
}

function defaultFilters(mode: IssueCollectionMode): IssueFilters {
  return mode === "board" ? defaultBoardView.filters : defaultListView.filters;
}

function parseLifecycle(value: string | null): IssueLifecycleFilter | undefined {
  return lifecycleValues.find((candidate) => candidate === value);
}

function parseValue<T>(
  values: readonly (readonly [string, T])[],
  encoded: string | null,
): T | undefined {
  return values.find(([value]) => value === encoded)?.[1];
}

function setMappedValue<T>(
  search: URLSearchParams,
  name: string,
  values: readonly (readonly [string, T])[],
  value: T | "all",
  defaultValue: T | "all",
): void {
  if (value === defaultValue) {
    return;
  }
  const encoded = values.find(([, candidate]) => candidate === value)?.[0];
  if (encoded !== undefined) {
    search.set(name, encoded);
  }
}

function setTextValue(
  search: URLSearchParams,
  name: string,
  value: string,
  defaultValue: string,
): void {
  const normalized = value.trim();
  if (normalized !== defaultValue) {
    search.set(name, normalized);
  }
}
