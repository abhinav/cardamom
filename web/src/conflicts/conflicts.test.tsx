// @vitest-environment jsdom

import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ConflictInspectionSchema,
  ConflictService,
  ConflictValueSchema,
  type GetConflictAlternativeRequest,
  type ResolveConflictRequest,
} from "../gen/cardamom/private/v1/conflicts_pb.ts";
import { IssueStatus, IssueType } from "../gen/cardamom/private/v1/issue_pb.ts";
import { ConflictsRoute } from "./conflicts.tsx";
import { ConflictValueContent } from "./conflict-values.tsx";
import {
  ExecutionRepairForm,
  RelationsRepairForm,
  relationRepairInput,
} from "./repairs.tsx";
import { conflictHref } from "./conflict-link.tsx";
import { SourceRefSchema } from "../gen/cardamom/private/v1/source_pb.ts";

afterEach(cleanup);

describe("conflict recovery", () => {
  it("loads bodies individually and requires a refresh after a stale review", async () => {
    const summary = {
      id: "reviewed",
      unit: { kind: "issue.title.set" },
      alternatives: 2,
    };
    let stale = false;
    const listConflicts = vi.fn(() => ({ conflicts: [summary] }));
    const getConflictAlternative = vi.fn(
      (_request: GetConflictAlternativeRequest) => {
        if (stale)
          throw new ConnectError("old value disappeared", Code.NotFound);
        return {
          value: {
            unit: summary.unit,
            value: { case: "text" as const, value: { text: "First title" } },
          },
        };
      },
    );
    const resolveConflict = vi.fn((_request: ResolveConflictRequest) => {
      stale = true;
      throw new ConnectError(
        "conflict changed; inspect current alternatives",
        Code.FailedPrecondition,
      );
    });
    const transport = createRouterTransport(({ service }) => {
      service(ConflictService, {
        listConflicts,
        getConflict: () => {
          if (stale)
            throw new ConnectError("old review disappeared", Code.NotFound);
          return {
            conflict: {
              summary,
              alternatives: [
                { id: "alt-a", unit: summary.unit },
                { id: "alt-b", unit: summary.unit },
              ],
            },
          };
        },
        getConflictAlternative,
        resolveConflict,
      });
    });
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    render(
      <QueryClientProvider client={client}>
        <TransportProvider transport={transport}>
          <MemoryRouter
            initialEntries={["/board/board-1/conflicts?conflict=reviewed"]}
          >
            <ConflictsRoute boardId="board-1" canMutate />
          </MemoryRouter>
        </TransportProvider>
      </QueryClientProvider>,
    );

    fireEvent.click(
      await screen.findByRole("button", { name: "Read value 1" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Use value 1" }));
    await screen.findByText(
      "This conflict changed or was resolved. Refresh the list before reviewing it again.",
    );
    expect(getConflictAlternative).toHaveBeenCalledTimes(1);
    expect(getConflictAlternative.mock.calls[0]?.[0]).toMatchObject({
      conflictId: "reviewed",
      alternativeId: "alt-a",
    });
    expect(resolveConflict.mock.calls[0]?.[0]).toMatchObject({
      conflictId: "reviewed",
      choice: { case: "take", value: "alt-a" },
    });
    await client.refetchQueries({ type: "all" });
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Use value 1" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry value" })).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Refresh conflict list" }),
    );
    await screen.findByRole("button", { name: "Inspect" });
    expect(resolveConflict).toHaveBeenCalledTimes(1);
  });

  it.each(["inspection", "alternative"] as const)(
    "freezes the review when %s reports stale before a mutation",
    async (source) => {
      let stale = false;
      const summary = {
        id: "reviewed",
        unit: { kind: "issue.title.set" },
        alternatives: 1,
      };
      const resolveConflict = vi.fn();
      const transport = createRouterTransport(({ service }) => {
        service(ConflictService, {
          listConflicts: () => ({ conflicts: [summary] }),
          getConflict: () => {
            if (stale && source === "inspection")
              throw new ConnectError(
                "conflict_reviewed_no_longer_exists",
                Code.NotFound,
              );
            return {
              conflict: {
                summary,
                alternatives: [{ id: "alt-a", unit: summary.unit }],
              },
            };
          },
          getConflictAlternative: () => {
            throw new ConnectError("value_no_longer_exists", Code.NotFound);
          },
          resolveConflict,
        });
      });
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      render(
        <QueryClientProvider client={client}>
          <TransportProvider transport={transport}>
            <MemoryRouter
              initialEntries={["/board/board-1/conflicts?conflict=reviewed"]}
            >
              <ConflictsRoute boardId="board-1" canMutate />
            </MemoryRouter>
          </TransportProvider>
        </QueryClientProvider>,
      );
      const read = await screen.findByRole("button", { name: "Read value 1" });
      if (source === "inspection") {
        fireEvent.click(
          screen.getByRole("button", { name: "Compose replacement text" }),
        );
        fireEvent.change(
          screen.getByRole("textbox", { name: "Complete replacement text" }),
          { target: { value: "Unsaved composition" } },
        );
      }
      stale = true;
      if (source === "inspection") await client.refetchQueries();
      else fireEvent.click(read);
      await screen.findByText(
        "This conflict changed or was resolved. Refresh the list before reviewing it again.",
      );
      expect(screen.getAllByRole("alert")).toHaveLength(1);
      expect(
        (
          screen.getByRole("button", {
            name: "Compose replacement text",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(true);
      if (source === "inspection") {
        const draft = screen.getByRole("textbox", {
          name: "Complete replacement text",
        }) as HTMLTextAreaElement;
        expect(draft.value).toBe("Unsaved composition");
        expect(draft.readOnly).toBe(true);
        expect(
          (
            screen.getByRole("button", {
              name: "Use replacement text",
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(true);
      }
      expect(screen.queryByRole("button", { name: "Retry value" })).toBeNull();
      expect(resolveConflict).not.toHaveBeenCalled();
    },
  );

  it("retries transport failures without discarding the reviewed choices", async () => {
    let inspectionUnavailable = true;
    let valueUnavailable = true;
    const summary = {
      id: "reviewed",
      unit: { kind: "issue.title.set" },
      alternatives: 1,
    };
    const transport = createRouterTransport(({ service }) => {
      service(ConflictService, {
        listConflicts: () => ({ conflicts: [summary] }),
        getConflict: () => {
          if (inspectionUnavailable)
            throw new ConnectError(
              "inspection transport unavailable",
              Code.Unavailable,
            );
          return {
            conflict: {
              summary,
              alternatives: [{ id: "alt-a", unit: summary.unit }],
            },
          };
        },
        getConflictAlternative: () => {
          if (valueUnavailable)
            throw new ConnectError(
              "value transport unavailable",
              Code.Unavailable,
            );
          return {
            value: {
              unit: summary.unit,
              value: { case: "text", value: { text: "Retained title" } },
            },
          };
        },
      });
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <TransportProvider transport={transport}>
          <MemoryRouter
            initialEntries={["/board/board-1/conflicts?conflict=reviewed"]}
          >
            <ConflictsRoute boardId="board-1" canMutate />
          </MemoryRouter>
        </TransportProvider>
      </QueryClientProvider>,
    );
    const retry = await screen.findByRole("button", {
      name: "Retry inspection",
    });
    inspectionUnavailable = false;
    fireEvent.click(retry);
    fireEvent.click(
      await screen.findByRole("button", { name: "Read value 1" }),
    );
    const retryValue = await screen.findByRole("button", {
      name: "Retry value",
    });
    valueUnavailable = false;
    fireEvent.click(retryValue);
    await screen.findByText("Retained title");
    expect(
      (screen.getByRole("button", { name: "Use value 1" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
    expect(
      screen.queryByRole("button", { name: "Refresh conflict list" }),
    ).toBeNull();
  });

  it("retains public reference navigation, previews, and copy controls", () => {
    const source = create(SourceRefSchema, { sourceId: "remote-source" });
    const value = create(ConflictValueSchema, {
      unit: { kind: "issue.dependency" },
      issues: [
        {
          uid: "private-a",
          issue: {
            id: "cm-child",
            boardId: "board-1",
            source,
            title: "Child task",
            status: IssueStatus.READY,
            type: IssueType.TASK,
          },
        },
        {
          uid: "private-b",
          issue: {
            id: "cm-parent",
            boardId: "board-1",
            source,
            title: "Prerequisite",
            status: IssueStatus.CLOSED,
            type: IssueType.TASK,
          },
        },
      ],
      value: {
        case: "membership",
        value: {
          present: true,
          issueUid: "private-a",
          relatedUid: "private-b",
        },
      },
    });
    const { container } = render(<ConflictValueContent value={value} />);
    expect(
      screen.getByRole("link", { name: "cm-child" }).getAttribute("href"),
    ).toBe("/source/remote-source/board/board-1/issue/cm-child");
    expect(container.querySelectorAll(".clipboard-pill").length).toBe(2);
    fireEvent.focus(screen.getByRole("link", { name: "cm-child" }));
    expect(screen.getByText("Child task")).toBeTruthy();
    expect(conflictHref("board-1", source, "cm-child")).toBe(
      "/source/remote-source/board/board-1/conflicts?issue=cm-child",
    );
  });

  it("submits a compound parent repair without inferring unparent from blank input", () => {
    expect(() =>
      relationRepairInput([], [{ child: "cm-child", parent: "", action: "" }]),
    ).toThrow("Choose a child");
    expect(
      relationRepairInput(
        ["dependency-value"],
        [
          { child: "cm-child", parent: "cm-parent", action: "set" },
          { child: "cm-other", parent: "", action: "remove" },
        ],
      ),
    ).toEqual({
      remove: ["dependency-value"],
      parents: [
        { childId: "cm-child", parentId: "cm-parent" },
        { childId: "cm-other" },
      ],
    });
  });

  it("submits the unchanged reviewed relationship set explicitly", () => {
    const submit = vi.fn();
    render(
      <RelationsRepairForm
        inspection={create(ConflictInspectionSchema)}
        disabled={false}
        submit={submit}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Apply relationship repair" }),
    );
    expect(submit).toHaveBeenCalledWith({ remove: [], parents: [] });
  });

  it("requires a waiting choice and never supplies a default type or lifecycle", async () => {
    const submit = vi.fn();
    render(
      <ExecutionRepairForm
        inspection={create(ConflictInspectionSchema)}
        disabled={false}
        submit={submit}
      />,
    );
    expect(
      (
        screen.getByRole("button", {
          name: "Apply execution repair",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("combobox", { name: "Waiting decision" }));
    const choice = await screen.findByRole("option", { name: "Clear waiting" });
    fireEvent.pointerDown(choice, { pointerType: "mouse" });
    fireEvent.click(choice);
    fireEvent.click(
      screen.getByRole("button", { name: "Apply execution repair" }),
    );
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({
        waiting: "clear",
        waitingAssignment: "",
        kind: undefined,
        lifecycle: undefined,
      }),
    );
  });
});
