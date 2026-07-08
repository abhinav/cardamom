import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";

import {
  HierarchyNodeSchema,
  RelatedIssueSchema,
  IssueStatus,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { buildHierarchyProjection } from "./hierarchy.tsx";

describe("buildHierarchyProjection", () => {
  it("builds one uncut rail over semantic issue rows", () => {
    const nodes = [
      hierarchyNode("cm-root", undefined, 0),
      hierarchyNode("cm-selected", "cm-root", 1, IssueStatus.IN_PROGRESS),
      hierarchyNode("cm-child", "cm-selected", 2),
      hierarchyNode("cm-sibling", "cm-root", 1),
    ];

    const projection = buildHierarchyProjection(nodes, "cm-selected");

    expect(projection.rows).toEqual([
      expect.objectContaining({
        issue: expect.objectContaining({ id: "cm-root" }),
        current: false,
      }),
      expect.objectContaining({
        issue: expect.objectContaining({
          id: "cm-selected",
          status: IssueStatus.IN_PROGRESS,
        }),
        current: true,
      }),
      expect.objectContaining({
        issue: expect.objectContaining({ id: "cm-child" }),
        current: false,
      }),
      expect.objectContaining({
        issue: expect.objectContaining({ id: "cm-sibling" }),
        current: false,
      }),
    ]);
    expect(projection.rail).toEqual({
      height: 176,
      width: 72,
      path: "M 12 22 V 66 H 32 M 32 66 V 110 H 52 M 12 22 V 154 H 32",
    });
  });
});

function hierarchyNode(
  id: string,
  parentId: string | undefined,
  depth: number,
  status = IssueStatus.READY,
) {
  return create(HierarchyNodeSchema, {
    issue: create(RelatedIssueSchema, {
      id,
      boardId: "board-1",
      title: `Title for ${id}`,
      status,
    }),
    parentId,
    depth,
  });
}
