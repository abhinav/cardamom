import { createContext, useContext, type ReactNode } from "react";

import { AccessMode } from "./gen/cardamom/private/v1/project_pb.ts";

interface ServerAccess {
  canMutateServer: boolean;
}

const ServerAccessContext = createContext<ServerAccess>({
  canMutateServer: false,
});

/** ServerAccessProvider translates bootstrap policy into application capability. */
export function ServerAccessProvider({
  accessMode,
  children,
}: {
  accessMode: AccessMode;
  children?: ReactNode;
}) {
  return (
    <ServerAccessContext.Provider
      value={{ canMutateServer: accessMode === AccessMode.READ_WRITE }}
    >
      {children}
    </ServerAccessContext.Provider>
  );
}

export function useServerAccess(): ServerAccess {
  return useContext(ServerAccessContext);
}

/** effectiveMutationCapability combines server policy with a local constraint. */
export function effectiveMutationCapability(
  canMutateServer: boolean,
  scopeAllowsMutations: boolean,
): boolean {
  return canMutateServer && scopeAllowsMutations;
}
