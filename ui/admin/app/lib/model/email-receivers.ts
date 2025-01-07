import { EntityMeta } from "~/lib/model/primitives";

type EmailReceiverBase = {
    name: string;
    description: string;
    alias?: string;
    workflow: string;
    allowedSenders?: string[];
};

export type EmailReceiver = EntityMeta & EmailReceiverBase;

export type CreateEmailReceiver = EmailReceiverBase;
export type UpdateEmailReceiver = EmailReceiverBase;
