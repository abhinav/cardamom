import {
  createClient,
  type Client,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { AttachmentService } from "./gen/cardamom/private/v1/attachment_pb.ts";
import { ChangeService } from "./gen/cardamom/private/v1/change_pb.ts";
import { CheckpointService } from "./gen/cardamom/private/v1/checkpoint_pb.ts";
import { ExecutionService } from "./gen/cardamom/private/v1/execution_pb.ts";
import { IssueService } from "./gen/cardamom/private/v1/issue_pb.ts";
import { PlanningService } from "./gen/cardamom/private/v1/planning_pb.ts";
import { ProjectService } from "./gen/cardamom/private/v1/project_pb.ts";
import { RecordService } from "./gen/cardamom/private/v1/record_pb.ts";

export type AttachmentClient = Client<typeof AttachmentService>;
export type ChangeClient = Client<typeof ChangeService>;
export type CheckpointClient = Client<typeof CheckpointService>;
export type ExecutionClient = Client<typeof ExecutionService>;
export type IssueClient = Client<typeof IssueService>;
export type PlanningClient = Client<typeof PlanningService>;
export type ProjectClient = Client<typeof ProjectService>;
export type RecordClient = Client<typeof RecordService>;

/** WebClient groups the generated domain clients sharing one Connect transport. */
export interface WebClient {
  /** attachments reads metadata and owns resumable upload mutations. */
  attachments: AttachmentClient;

  /** changes streams coarse invalidation notifications. */
  changes: ChangeClient;

  /** checkpoints reads and resolves actionable human decisions. */
  checkpoints: CheckpointClient;

  /** execution changes issue custody and lifecycle. */
  execution: ExecutionClient;

  /** issues reads issue collections and details. */
  issues: IssueClient;

  /** planning creates and edits issues and their relationships. */
  planning: PlanningClient;

  /** project loads startup metadata and project-owned board settings. */
  project: ProjectClient;

  /** records reads and changes issue log entries, state, and results. */
  records: RecordClient;
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
    checkpoints: createClient(CheckpointService, transport),
    execution: createClient(ExecutionService, transport),
    issues: createClient(IssueService, transport),
    planning: createClient(PlanningService, transport),
    project: createClient(ProjectService, transport),
    records: createClient(RecordService, transport),
  };
}
