import type {
  HierarchyNode,
  RelatedIssue,
} from "../gen/cardamom/private/v1/issue_pb.ts";
import { IssueReferenceLink } from "./issue-reference.tsx";

const hierarchyRowHeight = 44;
const hierarchyStartX = 12;
const hierarchyDepthStep = 20;

interface HierarchyRow {
  issue: RelatedIssue;
  current: boolean;
  depth: number;
}

interface HierarchyRail {
  height: number;
  width: number;
  path: string;
}

interface HierarchyProjection {
  rows: HierarchyRow[];
  rail: HierarchyRail;
}

/**
 * buildHierarchyProjection keeps semantic rows and one full-height decorative
 * rail aligned from the same preordered nodes.
 */
export function buildHierarchyProjection(
  nodes: readonly HierarchyNode[],
  selectedIssueId: string,
): HierarchyProjection {
  const presentNodes = nodes.filter(
    (node): node is HierarchyNode & { issue: RelatedIssue } =>
      node.issue !== undefined,
  );
  const indexByIssueID = new Map(
    presentNodes.map((node, index) => [node.issue.id, index]),
  );
  const rows = presentNodes.map((node) => ({
    issue: node.issue,
    current: node.issue.id === selectedIssueId,
    depth: node.depth,
  }));
  const segments: string[] = [];

  for (const [index, node] of presentNodes.entries()) {
    if (node.parentId === undefined) {
      continue;
    }
    const parentIndex = indexByIssueID.get(node.parentId);
    if (parentIndex === undefined) {
      continue;
    }
    const parent = presentNodes[parentIndex];
    if (parent === undefined) {
      continue;
    }
    const parentX = hierarchyStartX + parent.depth * hierarchyDepthStep;
    const parentY = parentIndex * hierarchyRowHeight + hierarchyRowHeight / 2;
    const childX = hierarchyStartX + node.depth * hierarchyDepthStep;
    const childY = index * hierarchyRowHeight + hierarchyRowHeight / 2;
    segments.push(`M ${parentX} ${parentY} V ${childY} H ${childX}`);
  }

  const maximumDepth = presentNodes.reduce(
    (maximum, node) => Math.max(maximum, node.depth),
    0,
  );
  return {
    rows,
    rail: {
      height: presentNodes.length * hierarchyRowHeight,
      width: hierarchyStartX + maximumDepth * hierarchyDepthStep + 20,
      path: segments.join(" "),
    },
  };
}

interface IssueHierarchyProps {
  nodes: readonly HierarchyNode[];
  selectedIssueId: string;
}

/** IssueHierarchy renders linked rows over one non-semantic SVG rail. */
export function IssueHierarchy({
  nodes,
  selectedIssueId,
}: IssueHierarchyProps) {
  const projection = buildHierarchyProjection(nodes, selectedIssueId);
  if (projection.rows.length === 0) {
    return <p className="issue-detail-empty">No containment hierarchy.</p>;
  }

  return (
    <div
      className="issue-hierarchy"
      style={{ minHeight: `${projection.rail.height}px` }}
    >
      <svg
        className="issue-hierarchy-rail"
        aria-hidden="true"
        focusable="false"
        width={projection.rail.width}
        height={projection.rail.height}
        viewBox={`0 0 ${projection.rail.width} ${projection.rail.height}`}
      >
        <path d={projection.rail.path} />
      </svg>
      <ol>
        {projection.rows.map((row) => (
          <li
            key={row.issue.id}
            style={{
              minHeight: `${hierarchyRowHeight}px`,
              paddingInlineStart: `${hierarchyStartX + row.depth * hierarchyDepthStep + 14}px`,
            }}
          >
            <IssueReferenceLink issue={row.issue} current={row.current} />
          </li>
        ))}
      </ol>
    </div>
  );
}
