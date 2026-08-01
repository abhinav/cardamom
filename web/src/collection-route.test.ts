import { describe, expect, it } from "vitest";

import { IssueStatus, IssueType } from "./gen/cardamom/private/v1/issue_pb.ts";
import {
  collectionRouteSearch,
  issueFiltersFromSearch,
  issueViewFromSearch,
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
