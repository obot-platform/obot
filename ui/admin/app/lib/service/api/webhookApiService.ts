import { CreateWebhook, UpdateWebhook, Webhook } from "~/lib/model/webhooks";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getWebhooks() {
    const { data } = await request<{ items: Webhook[] }>({
        url: ApiRoutes.webhooks.getWebhooks().url,
    });

    return data.items ?? [];
}
getWebhooks.key = () =>
    ({
        url: ApiRoutes.webhooks.getWebhooks().path,
    }) as const;

async function getWebhookById(webhookId: string) {
    const { data } = await request<Webhook>({
        url: ApiRoutes.webhooks.getWebhookById(webhookId).url,
    });

    return data;
}
getWebhookById.key = (id?: Nullish<string>) => {
    if (!id) return null;

    return {
        url: ApiRoutes.webhooks.getWebhookById(id).path,
        id,
    } as const;
};

async function createWebhook(payload: CreateWebhook) {
    const { data } = await request<Webhook>({
        url: ApiRoutes.webhooks.createWebhook().url,
        method: "POST",
        data: payload,
    });

    return data;
}

async function updateWebhook(webhookId: string, payload: UpdateWebhook) {
    const { data } = await request<Webhook>({
        url: ApiRoutes.webhooks.updateWebhook(webhookId).url,
        method: "PUT",
        data: payload,
    });

    return data;
}

async function removeWebhookToken(webhookId: string) {
    const { data } = await request<Webhook>({
        url: ApiRoutes.webhooks.removeWebhookToken(webhookId).url,
        method: "POST",
    });

    return data;
}

async function deleteWebhook(webhookId: string) {
    await request({
        url: ApiRoutes.webhooks.deleteWebhook(webhookId).url,
        method: "DELETE",
    });
}

export const WebhookApiService = {
    getWebhooks,
    getWebhookById,
    createWebhook,
    updateWebhook,
    deleteWebhook,
    removeWebhookToken,
};
