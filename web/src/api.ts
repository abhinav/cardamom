import {
  createClient,
  type Client,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { AttachmentService } from "./gen/cardamom/private/v1/attachment_pb.ts";
import { ChangeService } from "./gen/cardamom/private/v1/change_pb.ts";

export type AttachmentClient = Client<typeof AttachmentService>;
export type ChangeClient = Client<typeof ChangeService>;

/** WebClient groups the generated domain clients sharing one Connect transport. */
export interface WebClient {
  /** attachments reads metadata and owns resumable upload mutations. */
  attachments: AttachmentClient;

  /** changes streams coarse invalidation notifications. */
  changes: ChangeClient;
}

/** createWebTransport establishes the application-wide Connect transport. */
export function createWebTransport(
  baseUrl = window.location.origin,
): Transport {
  return createConnectTransport({ baseUrl });
}

/** createWebClient establishes generated domain clients over a shared transport. */
export function createWebClient(transport: Transport): WebClient {
  return {
    attachments: createClient(AttachmentService, transport),
    changes: createClient(ChangeService, transport),
  };
}
