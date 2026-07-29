import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  ClipboardPill,
  copyClipboardText,
  copyDocumentText,
} from "./clipboard-pill.tsx";

describe("clipboard pill", () => {
  it("presents separate content and an accessible copy control", () => {
    const markup = renderToStaticMarkup(
      <ClipboardPill
        copyLabel="Copy issue ID %cm-task"
        copyText="%cm-task"
      >
        <a href="/issues/cm-task">%cm-task</a>
      </ClipboardPill>,
    );

    expect(markup).toContain('class="clipboard-pill"');
    expect(markup).toContain('<a href="/issues/cm-task">%cm-task</a>');
    expect(markup).toContain('aria-label="Copy issue ID %cm-task"');
    expect(markup).toContain('title="Copy issue ID %cm-task"');
    expect(markup).toContain('role="status"');
    expect(markup).toContain('aria-live="polite"');
  });

  it("copies only the supplied text", async () => {
    const writeText = vi.fn(async () => undefined);
    const fallback = vi.fn(() => true);

    await expect(
      copyClipboardText({ writeText }, "%cm-task", fallback),
    ).resolves.toBe("copied");
    expect(writeText).toHaveBeenCalledWith("%cm-task");
    expect(fallback).not.toHaveBeenCalled();
  });

  it("falls back synchronously when the clipboard is unavailable", async () => {
    const fallback = vi.fn(() => true);

    const result = copyClipboardText(undefined, "%cm-task", fallback);

    expect(fallback).toHaveBeenCalledWith("%cm-task");
    await expect(result).resolves.toBe("copied");
  });

  it("reports a failed fallback", async () => {
    await expect(
      copyClipboardText(undefined, "%cm-task", () => false),
    ).resolves.toBe("failed");
  });

  it("copies through a one-shot native copy event", () => {
    const setData = vi.fn();
    const preventDefault = vi.fn();
    let copyListener: ((event: ClipboardEvent) => void) | undefined;
    const copyDocument = {
      addEventListener: vi.fn(
        (_type: "copy", listener: (event: ClipboardEvent) => void) => {
          copyListener = listener;
        },
      ),
      removeEventListener: vi.fn(),
      execCommand: vi.fn(() => {
        copyListener?.({
          clipboardData: { setData },
          preventDefault,
        } as unknown as ClipboardEvent);
        return true;
      }),
    };

    expect(copyDocumentText("%cm-task", copyDocument)).toBe(true);
    expect(setData).toHaveBeenCalledWith("text/plain", "%cm-task");
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(copyDocument.addEventListener).toHaveBeenCalledWith(
      "copy",
      copyListener,
      { once: true },
    );
    expect(copyDocument.removeEventListener).toHaveBeenCalledWith(
      "copy",
      copyListener,
    );
  });

  it("reports success only when the command and payload both succeed", () => {
    let copyListener: ((event: ClipboardEvent) => void) | undefined;
    const copyDocument = {
      addEventListener: vi.fn(
        (_type: "copy", listener: (event: ClipboardEvent) => void) => {
          copyListener = listener;
        },
      ),
      removeEventListener: vi.fn(),
      execCommand: vi.fn(() => {
        copyListener?.({
          clipboardData: { setData: vi.fn() },
          preventDefault: vi.fn(),
        } as unknown as ClipboardEvent);
        return false;
      }),
    };

    expect(copyDocumentText("%cm-task", copyDocument)).toBe(false);
    expect(copyDocument.removeEventListener).toHaveBeenCalledWith(
      "copy",
      copyListener,
    );

    copyDocument.execCommand.mockImplementation(() => true);
    expect(copyDocumentText("%cm-task", copyDocument)).toBe(false);
    expect(copyDocument.removeEventListener).toHaveBeenCalledTimes(2);
  });
});
