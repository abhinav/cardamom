import { ClipboardPill } from "./clipboard-pill.tsx";
import type { Attachment } from "./gen/cardamom/private/v1/attachment_pb.ts";

import "./attachment-identity.css";

type AttachmentReference = Pick<Attachment, "filename" | "id">;

/** AttachmentIdentity presents a filename and its copyable attachment ID. */
export function AttachmentIdentity({
  attachment,
}: {
  attachment: AttachmentReference;
}) {
  return (
    <div className="attachment-identity">
      <strong>{attachment.filename}</strong>
      <ClipboardPill
        copyLabel={`Copy attachment ID ${attachment.id}`}
        copyText={attachment.id}
      >
        <code>{attachment.id}</code>
      </ClipboardPill>
    </div>
  );
}
