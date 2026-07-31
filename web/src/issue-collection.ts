import { create } from "@bufbuild/protobuf";

import {
  IssueLifecycle,
  IssueSort,
  IssueStatus,
  IssueType,
  SortDirection,
  type IssueSummary,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  ListIssuesRequestSchema,
  type ListIssuesRequest,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import type { BoardScope } from "./gen/cardamom/private/v1/scope_pb.ts";
import { issueStatusPresentation } from "./issue-status.tsx";

export type IssueGrouping = "status" | "type";
/** "current" includes open and closed work while excluding cancelled work. */
export type IssueLifecycleFilter =
  | "current"
  | "open"
  | "closed"
  | "cancelled"
  | "all";
/** "natural" orders each work state by the signal most useful to an observer. */
export type IssueSortPreference =
  | "natural"
  | "priority"
  | "updated"
  | "created"
  | "title";
export type SortDirectionPreference = "ascending" | "descending";

export interface IssueFilters {
  lifecycle: IssueLifecycleFilter;
  status: IssueStatus | "all";
  /** "all" selects ordinary work; routines require an explicit selection. */
  type: IssueType | "all";
  actor: string;
  label: string;
  query: string;
}

export interface IssueViewPreferences {
  filters: IssueFilters;
  sort: IssueSortPreference;
  direction: SortDirectionPreference;
}

export interface BoardViewPreferences extends IssueViewPreferences {
  grouping: IssueGrouping;
  showEmptyColumns: boolean;
}

export interface IssueGroup {
  key: string;
  label: string;
  issues: readonly IssueSummary[];
}

export const defaultBoardView: BoardViewPreferences = {
  grouping: "status",
  showEmptyColumns: false,
  filters: {
    lifecycle: "current",
    status: "all",
    type: "all",
    actor: "",
    label: "",
    query: "",
  },
  sort: "natural",
  direction: "descending",
};

export const defaultListView: IssueViewPreferences = {
  filters: {
    lifecycle: "all",
    status: "all",
    type: "all",
    actor: "",
    label: "",
    query: "",
  },
  sort: "updated",
  direction: "descending",
};

/** clearIssueFilters restores every filter to its route default. */
export function clearIssueFilters(mode: "board" | "list"): IssueFilters {
  const defaults = mode === "board"
    ? defaultBoardView.filters
    : defaultListView.filters;
  return { ...defaults };
}

/** listViewForLabel opens one complete label collection in the existing order. */
export function listViewForLabel(
  current: IssueViewPreferences,
  label: string,
): IssueViewPreferences {
  return {
    ...defaultListView,
    filters: { ...defaultListView.filters, label: label.trim() },
    sort: current.sort,
    direction: current.direction,
  };
}

/** buildIssueQuery is the only translation from browser preferences to ListIssues. */
export function buildIssueQuery(
  scope: BoardScope,
  view: IssueViewPreferences,
): ListIssuesRequest {
  const actor = view.filters.actor.trim();
  const label = view.filters.label.trim();
  const titleQuery = view.filters.query.trim();
  return create(ListIssuesRequestSchema, {
    scope,
    lifecycles: lifecycleValues(view.filters.lifecycle),
    statuses: view.filters.status === "all" ? [] : [view.filters.status],
    types:
      view.filters.type === "all"
        ? [...ordinaryIssueTypes]
        : [view.filters.type],
    ...(actor === "" ? {} : { actor }),
    labelsAll: label === "" ? [] : [label],
    ...(titleQuery === "" ? {} : { titleQuery }),
    sort: sortValue(view.sort),
    direction:
      view.sort !== "natural" && view.direction === "ascending"
        ? SortDirection.ASCENDING
        : SortDirection.DESCENDING,
    limit: 100,
  });
}

export function groupIssues(
  issues: readonly IssueSummary[],
  grouping: IssueGrouping,
  lifecycle: IssueLifecycleFilter,
  selectedType: IssueType | "all" = "all",
  sort: IssueSortPreference = "natural",
  direction: SortDirectionPreference = "ascending",
): IssueGroup[] {
  if (grouping === "status") {
    return statusValues(lifecycle).map((status) => ({
      key: `status:${status}`,
      label: issueStatusPresentation(status).label,
      issues: orderIssues(
        issues.filter((issue) => issue.status === status),
        sort,
        direction,
      ),
    }));
  }
  const groupTypes =
    selectedType === "all" ? ordinaryIssueTypes : [selectedType];
  return groupTypes.map((type) => ({
    key: `type:${type}`,
    label: issueTypeLabel(type),
    issues: orderIssues(
      issues.filter((issue) => issue.type === type),
      sort,
      direction,
    ),
  }));
}

export function issueTypeLabel(type: IssueType): string {
  switch (type) {
    case IssueType.WORKSTREAM:
      return "Workstream";
    case IssueType.TASK:
      return "Task";
    case IssueType.CHECKPOINT:
      return "Checkpoint";
    case IssueType.ROUTINE:
      return "Routine";
    default:
      return "Unknown";
  }
}

export function parseBoardView(value: unknown): BoardViewPreferences {
  if (!isRecord(value)) {
    return defaultBoardView;
  }
  const shared = parseIssueView(value, defaultBoardView);
  return {
    ...shared,
    grouping: isGrouping(value.grouping)
      ? value.grouping
      : defaultBoardView.grouping,
    showEmptyColumns: typeof value.showEmptyColumns === "boolean"
      ? value.showEmptyColumns
      : defaultBoardView.showEmptyColumns,
  };
}

export function parseListView(value: unknown): IssueViewPreferences {
  return parseIssueView(value, defaultListView);
}

/** ordinaryIssueTypes keeps routine work in its dedicated view by default. */
const ordinaryIssueTypes = [
  IssueType.WORKSTREAM,
  IssueType.TASK,
  IssueType.CHECKPOINT,
] as const;

const selectableIssueTypes = [
  ...ordinaryIssueTypes,
  IssueType.ROUTINE,
] as const;

function lifecycleValues(filter: IssueLifecycleFilter): IssueLifecycle[] {
  switch (filter) {
    case "current":
      return [IssueLifecycle.OPEN, IssueLifecycle.CLOSED];
    case "open":
      return [IssueLifecycle.OPEN];
    case "closed":
      return [IssueLifecycle.CLOSED];
    case "cancelled":
      return [IssueLifecycle.CANCELLED];
    case "all":
      return [];
  }
}

/** visibleIssueStatuses returns the status columns allowed by the active filters. */
export function visibleIssueStatuses(
  lifecycle: IssueLifecycleFilter,
  selectedStatus: IssueStatus | "all" = "all",
): IssueStatus[] {
  const statuses = statusValues(lifecycle);
  return selectedStatus === "all"
    ? statuses
    : statuses.filter((status) => status === selectedStatus);
}

/** visibleIssueTypes returns the type columns allowed by the active filter. */
export function visibleIssueTypes(
  selectedType: IssueType | "all",
): readonly IssueType[] {
  return selectedType === "all" ? ordinaryIssueTypes : [selectedType];
}

function statusValues(filter: IssueLifecycleFilter): IssueStatus[] {
  switch (filter) {
    case "current":
      return [
        IssueStatus.READY,
        IssueStatus.BLOCKED,
        IssueStatus.IN_PROGRESS,
        IssueStatus.WAITING,
        IssueStatus.CLOSED,
      ];
    case "open":
      return [
        IssueStatus.READY,
        IssueStatus.BLOCKED,
        IssueStatus.IN_PROGRESS,
        IssueStatus.WAITING,
      ];
    case "closed":
      return [IssueStatus.CLOSED];
    case "cancelled":
      return [IssueStatus.CANCELLED];
    case "all":
      return [
        IssueStatus.READY,
        IssueStatus.BLOCKED,
        IssueStatus.IN_PROGRESS,
        IssueStatus.WAITING,
        IssueStatus.CLOSED,
        IssueStatus.CANCELLED,
      ];
  }
}

function sortValue(sort: IssueSortPreference): IssueSort {
  switch (sort) {
    case "natural":
      return IssueSort.UPDATED_AT;
    case "priority":
      return IssueSort.PRIORITY;
    case "updated":
      return IssueSort.UPDATED_AT;
    case "created":
      return IssueSort.CREATED_AT;
    case "title":
      return IssueSort.TITLE;
  }
}

function parseIssueView(
  value: unknown,
  fallback: IssueViewPreferences,
): IssueViewPreferences {
  if (
    !isRecord(value) ||
    !isSort(value.sort) ||
    !isDirection(value.direction) ||
    !isFilters(value.filters)
  ) {
    return fallback;
  }
  return {
    filters: value.filters,
    sort: value.sort,
    direction: value.direction,
  };
}

function isFilters(value: unknown): value is IssueFilters {
  if (!isRecord(value)) {
    return false;
  }
  return (
    isLifecycle(value.lifecycle) &&
    (value.status === "all" || isIssueStatus(value.status)) &&
    (value.type === "all" || isIssueType(value.type)) &&
    typeof value.actor === "string" &&
    typeof value.label === "string" &&
    typeof value.query === "string"
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isLifecycle(value: unknown): value is IssueLifecycleFilter {
  return (
    value === "current" ||
    value === "open" ||
    value === "closed" ||
    value === "cancelled" ||
    value === "all"
  );
}

function isIssueStatus(value: unknown): value is IssueStatus {
  return (
    value === IssueStatus.READY ||
    value === IssueStatus.BLOCKED ||
    value === IssueStatus.IN_PROGRESS ||
    value === IssueStatus.WAITING ||
    value === IssueStatus.CLOSED ||
    value === IssueStatus.CANCELLED
  );
}

function isIssueType(value: unknown): value is IssueType {
  return selectableIssueTypes.some((type) => type === value);
}

function isGrouping(value: unknown): value is IssueGrouping {
  return value === "status" || value === "type";
}

function isSort(value: unknown): value is IssueSortPreference {
  return (
    value === "natural" ||
    value === "priority" ||
    value === "updated" ||
    value === "created" ||
    value === "title"
  );
}

function isDirection(value: unknown): value is SortDirectionPreference {
  return value === "ascending" || value === "descending";
}

function orderIssues(
  issues: readonly IssueSummary[],
  sort: IssueSortPreference,
  direction: SortDirectionPreference,
): IssueSummary[] {
  const ordered = [...issues];
  if (sort === "natural") {
    return ordered.sort(compareNaturalOrder);
  }
  const directionFactor = direction === "ascending" ? 1 : -1;
  return ordered.sort((left, right) => {
    const comparison = compareIssueField(left, right, sort);
    return comparison === 0
      ? left.id.localeCompare(right.id)
      : comparison * directionFactor;
  });
}

function compareNaturalOrder(left: IssueSummary, right: IssueSummary): number {
  const leftGroup = naturalOrderGroup(left.status);
  const rightGroup = naturalOrderGroup(right.status);
  if (leftGroup !== rightGroup) {
    return leftGroup - rightGroup;
  }
  if (leftGroup === 1) {
    const priority = left.priority - right.priority;
    if (priority !== 0) {
      return priority;
    }
  } else {
    const updated = compareTimestamp(right.updatedAt, left.updatedAt);
    if (updated !== 0) {
      return updated;
    }
  }
  return left.id.localeCompare(right.id);
}

function naturalOrderGroup(status: IssueStatus): number {
  switch (status) {
    case IssueStatus.IN_PROGRESS:
      return 0;
    case IssueStatus.READY:
    case IssueStatus.BLOCKED:
      return 1;
    case IssueStatus.WAITING:
      return 2;
    case IssueStatus.CLOSED:
      return 3;
    case IssueStatus.CANCELLED:
    default:
      return 4;
  }
}

function compareIssueField(
  left: IssueSummary,
  right: IssueSummary,
  sort: Exclude<IssueSortPreference, "natural">,
): number {
  switch (sort) {
    case "priority":
      return left.priority - right.priority;
    case "updated":
      return compareTimestamp(left.updatedAt, right.updatedAt);
    case "created":
      return compareTimestamp(left.createdAt, right.createdAt);
    case "title":
      return left.title.localeCompare(right.title);
  }
}

function compareTimestamp(
  left: IssueSummary["updatedAt"],
  right: IssueSummary["updatedAt"],
): number {
  if (left === undefined) {
    return right === undefined ? 0 : -1;
  }
  if (right === undefined) {
    return 1;
  }
  const seconds = left.seconds < right.seconds ? -1 : left.seconds > right.seconds ? 1 : 0;
  return seconds === 0 ? left.nanos - right.nanos : seconds;
}
