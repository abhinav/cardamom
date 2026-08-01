import type { CSSProperties, ReactElement } from "react";
import { cloneElement, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";

import { ClipboardPill } from "./clipboard-pill.tsx";
import type {
  IssueStatus,
  IssueType,
} from "./gen/cardamom/private/v1/issue_pb.ts";
import { issueTypeLabel } from "./issue-collection.ts";
import {
  IssueStatusDot,
  issueStatusPresentation,
} from "./issue-status.tsx";

import "./issue-reference-pill.css";

/** IssueReferencePreview is the compact metadata needed to identify an issue. */
export interface IssueReferencePreview {
  /** title is the human-readable issue title. */
  title: string;

  /** status is the issue's effective workflow status. */
  status: IssueStatus;

  /** type identifies the issue's workflow role. */
  type: IssueType;

  /** priority is the issue's domain ordering priority. */
  priority: number;
}

/** LoadIssueReferencePreview obtains one issue preview through the shared cache. */
export type LoadIssueReferencePreview = (
  issueID: string,
) => Promise<IssueReferencePreview>;

/** IssueReferencePill presents a copyable issue ID with optional hover context. */
export function IssueReferencePill({
  children,
  issue,
  issueID,
  loadIssue,
}: {
  children: ReactElement<{ "aria-describedby"?: string }>;
  issue?: IssueReferencePreview;
  issueID: string;
  loadIssue?: LoadIssueReferencePreview;
}) {
  const [loadedIssue, setLoadedIssue] = useState<IssueReferencePreview>();
  const [hovering, setHovering] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const [previewPosition, setPreviewPosition] = useState<CSSProperties>();
  const loading = useRef(false);
  const pill = useRef<HTMLSpanElement>(null);
  const previewID = useId();
  const preview = issue ?? loadedIssue;
  const status = preview === undefined
    ? undefined
    : issueStatusPresentation(preview.status);
  const visible = preview !== undefined && !dismissed && (hovering || focused);
  const reference = `%${issueID}`;

  const positionPreview = () => {
    if (pill.current === null || typeof window === "undefined") {
      return;
    }
    const bounds = pill.current.getBoundingClientRect();
    const margin = 16;
    const previewWidth = Math.min(304, window.innerWidth - 2 * margin);
    setPreviewPosition({
      left: Math.max(
        margin,
        Math.min(bounds.left, window.innerWidth - previewWidth - margin),
      ),
      top: bounds.bottom + 7,
    });
  };

  const requestPreview = () => {
    if (preview !== undefined || loadIssue === undefined || loading.current) {
      return;
    }
    loading.current = true;
    void loadIssue(issueID)
      .then(setLoadedIssue)
      .catch(() => undefined)
      .finally(() => {
        loading.current = false;
      });
  };

  return (
    <span
      className="issue-reference-pill"
      ref={pill}
      onPointerEnter={(event) => {
        if (!pointerCanPreview(event.pointerType, browserCanHover())) {
          return;
        }
        setDismissed(false);
        setHovering(true);
        positionPreview();
        requestPreview();
      }}
      onPointerLeave={() => setHovering(false)}
      onFocusCapture={(event) => {
        if (!(event.target instanceof HTMLElement) ||
          !event.target.matches(":focus-visible")) {
          return;
        }
        setDismissed(false);
        setFocused(true);
        positionPreview();
        requestPreview();
      }}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) {
          setFocused(false);
          setDismissed(false);
        }
      }}
      onKeyDownCapture={(event) => {
        if (event.key === "Escape") {
          setDismissed(true);
        }
      }}
    >
      <ClipboardPill
        copyLabel={`Copy issue ID ${reference}`}
        copyText={reference}
      >
        {cloneElement(children, {
          "aria-describedby": visible ? previewID : undefined,
        })}
      </ClipboardPill>
      {preview !== undefined && renderPreview(
        <span
          className="issue-reference-preview"
          hidden={!visible}
          id={previewID}
          role="tooltip"
          style={previewPosition}
        >
          <span className="issue-reference-preview-id">{reference}</span>
          <strong>{preview.title}</strong>
          <span className="issue-reference-preview-metadata">
            <span className="issue-reference-preview-status">
              <IssueStatusDot status={preview.status} />
              {status?.label}
            </span>
            <span>{issueTypeLabel(preview.type)}</span>
            <span>P{preview.priority}</span>
          </span>
        </span>,
        visible,
      )}
    </span>
  );
}

/** pointerCanPreview reports whether one pointer supports transient context. */
export function pointerCanPreview(
  pointerType: string,
  hoverCapable: boolean,
): boolean {
  return hoverCapable && pointerType !== "touch";
}

function browserCanHover(): boolean {
  return typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(hover: hover) and (pointer: fine)").matches;
}

function renderPreview(preview: ReactElement, visible: boolean): ReactElement {
  if (typeof document === "undefined" || !visible) {
    return preview;
  }
  return createPortal(preview, document.body);
}
