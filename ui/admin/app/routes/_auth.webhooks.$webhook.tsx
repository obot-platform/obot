import { ClientLoaderFunctionArgs, useLoaderData } from "@remix-run/react";
import useSWR, { preload } from "swr";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { RouteService } from "~/lib/service/routeService";

import { WebhookForm } from "~/components/webhooks/WebhookForm";

export async function clientLoader({
    request,
    params,
}: ClientLoaderFunctionArgs) {
    const { pathParams } = RouteService.getRouteInfo(
        "/webhooks/:webhook",
        new URL(request.url),
        params
    );

    await preload(
        WebhookApiService.getWebhookById.key(pathParams.webhook),
        () => WebhookApiService.getWebhookById(pathParams.webhook)
    );

    return { webhookId: pathParams.webhook };
}

export default function Webhook() {
    const { webhookId } = useLoaderData<typeof clientLoader>();

    const { data: webhook } = useSWR(
        WebhookApiService.getWebhookById.key(webhookId),
        ({ id }) => WebhookApiService.getWebhookById(id)
    );

    return <WebhookForm webhook={webhook} />;
}
