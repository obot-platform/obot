import { z } from "zod";

import { EntityMeta, MetaLinks, Metadata } from "~/lib/model/primitives";

export type WebhookBase = {
    name: string;
    description: string;
    alias?: Nullish<string>;
    workflow: string;
    headers?: Nullish<string[]>;
    secret?: string;
    validationHeader: string;
};

export type WebhookDetail = WebhookBase & {
    aliasAssigned: boolean;
    lastSuccessfulRunCompleted?: string; // date
    hasToken?: boolean;
};

type WebhookLinks = { invoke: string } & MetaLinks;

export type Webhook = EntityMeta<Metadata, WebhookLinks> & WebhookDetail;

type WebhookPayload = WebhookBase & {
    token: Nullish<string>;
};

export type CreateWebhook = WebhookPayload;
export type UpdateWebhook = WebhookPayload;

export const WebhookSchema = z.object({
    name: z.string().min(1, "Name is required").default(""),
    description: z.string().min(1, "Description is required").default(""),
    alias: z.string().default(""),
    workflow: z.string().min(1, "Workflow is required").default(""),
    headers: z.array(z.string()).default([]),
    secret: z.string().default(""),
    validationHeader: z
        .string()
        .min(1, "Validation header is required")
        .default(""),
    token: z.string().default(""),
});
export type WebhookFormType = z.infer<typeof WebhookSchema>;
