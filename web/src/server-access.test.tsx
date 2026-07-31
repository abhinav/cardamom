import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AccessMode } from "./gen/cardamom/private/v1/project_pb.ts";
import {
  effectiveMutationCapability,
  ServerAccessProvider,
  useServerAccess,
} from "./server-access.tsx";

describe("server access", () => {
  it("exposes mutations only for explicit bootstrap read-write mode", () => {
    expect(renderAccess(AccessMode.READ_WRITE)).toContain("enabled");
    expect(renderAccess(AccessMode.READ_ONLY)).toContain("disabled");
    expect(renderAccess(AccessMode.UNSPECIFIED)).toContain("disabled");
    expect(renderToStaticMarkup(<AccessProbe />)).toContain("disabled");
  });

  it("combines server access with independent scope constraints", () => {
    expect(effectiveMutationCapability(true, true)).toBe(true);
    expect(effectiveMutationCapability(false, true)).toBe(false);
    expect(effectiveMutationCapability(true, false)).toBe(false);
  });
});

function renderAccess(accessMode: AccessMode): string {
  return renderToStaticMarkup(
    <ServerAccessProvider accessMode={accessMode}>
      <AccessProbe />
    </ServerAccessProvider>,
  );
}

function AccessProbe() {
  const { canMutateServer } = useServerAccess();
  return <span>{canMutateServer ? "enabled" : "disabled"}</span>;
}
