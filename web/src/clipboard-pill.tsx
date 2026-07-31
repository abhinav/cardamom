import { Copy } from "lucide-react";
import { type ReactNode, useState } from "react";

import "./clipboard-pill.css";

type CopyStatus = "idle" | "copied" | "failed";

/** ClipboardWriter is the browser clipboard surface used by copy controls. */
export interface ClipboardWriter {
  /** writeText replaces the clipboard with the supplied text. */
  writeText(text: string): Promise<void>;
}

/** copyClipboardText reports whether the clipboard accepted the supplied text. */
export async function copyClipboardText(
  clipboard: ClipboardWriter | undefined,
  text: string,
  fallback: (text: string) => boolean = copyDocumentText,
): Promise<Exclude<CopyStatus, "idle">> {
  if (clipboard === undefined) {
    try {
      return fallback(text) ? "copied" : "failed";
    } catch {
      return "failed";
    }
  }
  try {
    await clipboard.writeText(text);
    return "copied";
  } catch {
    return "failed";
  }
}

/** ClipboardPill pairs inline content and optional hover context with a copy control. */
export function ClipboardPill({
  children,
  copyLabel,
  copyText,
  title,
}: {
  children: ReactNode;
  copyLabel: string;
  copyText: string;
  title?: string;
}) {
  const [status, setStatus] = useState<CopyStatus>("idle");
  return (
    <span className="clipboard-pill" data-copy-status={status} title={title}>
      <span className="clipboard-pill-content">{children}</span>
      <button
        type="button"
        aria-label={copyLabel}
        title={copyLabel}
        onClick={() => {
          void copyClipboardText(browserClipboard(), copyText).then(setStatus);
        }}
      >
        <Copy aria-hidden="true" />
      </button>
      <span
        className="sr-only"
        role={status === "failed" ? "alert" : "status"}
        aria-live="polite"
      >
        {status === "copied"
          ? `${copyText} copied.`
          : status === "failed"
            ? `Could not copy ${copyText}.`
            : ""}
      </span>
    </span>
  );
}

function browserClipboard(): ClipboardWriter | undefined {
  return typeof navigator === "undefined" ? undefined : navigator.clipboard;
}

interface CopyDocument {
  addEventListener(
    type: "copy",
    listener: (event: ClipboardEvent) => void,
    options: { once: true },
  ): void;
  removeEventListener(
    type: "copy",
    listener: (event: ClipboardEvent) => void,
  ): void;
  execCommand(command: "copy"): boolean;
}

/** copyDocumentText synchronously copies text during the initiating gesture. */
export function copyDocumentText(
  text: string,
  copyDocument: CopyDocument | undefined =
    typeof document === "undefined" ? undefined : document,
): boolean {
  if (copyDocument === undefined) {
    return false;
  }

  let accepted = false;
  const handleCopy = (event: ClipboardEvent) => {
    if (event.clipboardData === null) {
      return;
    }
    event.clipboardData.setData("text/plain", text);
    event.preventDefault();
    accepted = true;
  };
  copyDocument.addEventListener("copy", handleCopy, { once: true });
  try {
    return copyDocument.execCommand("copy") && accepted;
  } catch {
    return false;
  } finally {
    copyDocument.removeEventListener("copy", handleCopy);
  }
}
