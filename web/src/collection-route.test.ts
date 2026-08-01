import { describe, expect, it } from "vitest";

import { IssueStatus, IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  collectionRouteSearch,
  issueFiltersFromSearch,
  issueViewFromSearch,
  labelCollectionLocation,
  routineRetiredFromSearch,
  routineRetiredSearch,
} from "./collection-route.ts";
import { defaultBoardView, defaultListView } from "./issue-collection.ts";

describe("collection route filters", () => {
  it("reads the existing issue filters from readable query parameters", () => {
    const filters = issueFiltersFromSearch(
      new URLSearchParams(
        "lifecycle=open&status=in-progress&type=task&actor=Pascal&" +
          "label=area%3Aweb&title=route+filters",
      ),
      "board",
    );

    expect(filters).toEqual({
      lifecycle: "open",
      status: IssueStatus.IN_PROGRESS,
      type: IssueType.TASK,
      actor: "Pascal",
      label: "area:web",
      query: "route filters",
    });
  });

  it("uses route defaults for absent or unsupported values", () => {
    expect(
      issueFiltersFromSearch(
        new URLSearchParams("lifecycle=historic&status=unknown&type=unknown"),
        "board",
      ),
    ).toEqual(defaultBoardView.filters);
    expect(issueFiltersFromSearch(new URLSearchParams(), "list")).toEqual(
      defaultListView.filters,
    );
  });

  it("omits target defaults while preserving the effective filters across views", () => {
    expect(collectionRouteSearch(defaultBoardView.filters, "board")).toBe("");
    expect(collectionRouteSearch(defaultListView.filters, "list")).toBe("");

    expect(collectionRouteSearch(defaultBoardView.filters, "list")).toBe(
      "?lifecycle=current",
    );
    expect(collectionRouteSearch(defaultListView.filters, "board")).toBe(
      "?lifecycle=all",
    );
  });

  it("uses the current location for filters and preferences for presentation", () => {
    const view = issueViewFromSearch(
      {
        ...defaultBoardView,
        grouping: "type",
        sort: "title",
        direction: "ascending",
        filters: { ...defaultBoardView.filters, actor: "stale" },
      },
      new URLSearchParams("actor=Pascal&label=area%3Aweb"),
      "board",
    );

    expect(view).toEqual({
      ...defaultBoardView,
      grouping: "type",
      sort: "title",
      direction: "ascending",
      filters: {
        ...defaultBoardView.filters,
        actor: "Pascal",
        label: "area:web",
      },
    });
  });

  it("serializes only non-default result filters", () => {
    expect(
      collectionRouteSearch(
        {
          lifecycle: "closed",
          status: IssueStatus.CLOSED,
          type: IssueType.WORKSTREAM,
          actor: " Pascal ",
          label: " area:web ",
          query: " route filters ",
        },
        "list",
      ),
    ).toBe(
      "?lifecycle=closed&status=closed&type=workstream&actor=Pascal&" +
        "label=area%3Aweb&title=route+filters",
    );
  });

  it("keeps retired routine visibility in the URL", () => {
    expect(routineRetiredFromSearch(new URLSearchParams())).toBe(false);
    expect(routineRetiredFromSearch(new URLSearchParams("retired=true"))).toBe(
      true,
    );
    expect(routineRetiredFromSearch(new URLSearchParams("retired=false"))).toBe(
      false,
    );
    expect(routineRetiredSearch(false)).toBe("");
    expect(routineRetiredSearch(true)).toBe("?retired=true");
  });
});

describe("label collection navigation", () => {
  it("retains Board and List modes in board scope", () => {
    expect(
      labelCollectionLocation(
        "/board/board-1",
        " area:web ",
      ),
    ).toEqual({
      pathname: "/board/board-1",
      search: "?label=area%3Aweb",
    });
    expect(
      labelCollectionLocation(
        "/board/board-1/list",
        "area:web",
      ),
    ).toEqual({
      pathname: "/board/board-1/list",
      search: "?label=area%3Aweb",
    });
  });

  it("retains Board and List modes across all boards", () => {
    expect(labelCollectionLocation("/all", "area:web")).toEqual({
      pathname: "/all",
      search: "?label=area%3Aweb",
    });
    expect(labelCollectionLocation("/all/list", "area:web")).toEqual({
      pathname: "/all/list",
      search: "?label=area%3Aweb",
    });
  });

  it("opens scoped List routes from non-collection pages", () => {
    expect(
      labelCollectionLocation(
        "/board/board-2/issue/cm-task",
        "area:web",
      ),
    ).toEqual({
      pathname: "/board/board-2/list",
      search: "?label=area%3Aweb",
    });
    expect(
      labelCollectionLocation("/board/board-1/approvals", "area:web"),
    ).toEqual({
      pathname: "/board/board-1/list",
      search: "?label=area%3Aweb",
    });
    expect(labelCollectionLocation("/all/routines", "area:web")).toEqual({
      pathname: "/all/list",
      search: "?label=area%3Aweb",
    });
    expect(
      labelCollectionLocation("/board/board-1/settings", "area:web"),
    ).toEqual({
      pathname: "/board/board-1/list",
      search: "?label=area%3Aweb",
    });
  });

  it("does not infer scope outside canonical board routes", () => {
    expect(labelCollectionLocation("/", "area:web")).toBeUndefined();
    expect(labelCollectionLocation("/unknown", "area:web")).toBeUndefined();
  });

  it("makes the selected label visible through canonical controls", () => {
    const location = labelCollectionLocation(
      "/board/board-1",
      "area:web",
    );

    expect(location).toBeDefined();
    expect(
      issueFiltersFromSearch(
        new URLSearchParams(location?.search),
        "board",
      ),
    ).toEqual({
      ...defaultBoardView.filters,
      label: "area:web",
    });
  });
});
