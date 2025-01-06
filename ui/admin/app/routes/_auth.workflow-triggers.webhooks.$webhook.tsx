import {
    ClientLoaderFunctionArgs,
    MetaFunction,
    useLoaderData,
    useMatch,
} from "react-router";
import useSWR, { preload } from "swr";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";

import { WebhookForm } from "~/components/webhooks/WebhookForm";

export async function clientLoader({
    request,
    params,
}: ClientLoaderFunctionArgs) {
    const { pathParams } = RouteService.getRouteInfo(
        "/workflow-triggers/webhooks/:webhook",
        new URL(request.url),
        params
    );

    const webhook = await preload(
        WebhookApiService.getWebhookById.key(pathParams.webhook),
        () => WebhookApiService.getWebhookById(pathParams.webhook)
    );

    return { webhookId: pathParams.webhook, webhook };
}

export default function Webhook() {
    const { webhookId } = useLoaderData<typeof clientLoader>();

    const { data: webhook } = useSWR(
        WebhookApiService.getWebhookById.key(webhookId),
        ({ id }) => WebhookApiService.getWebhookById(id)
    );

    return <WebhookForm webhook={webhook} />;
}

const WebhookBreadcrumb = () => {
    const match = useMatch("/workflow-triggers/webhooks/:webhook");

    const { data: webhook } = useSWR(
        WebhookApiService.getWebhookById.key(match?.params.webhook || ""),
        ({ id }) => WebhookApiService.getWebhookById(id)
    );

    return webhook?.name || webhook?.id || "Edit";
};

export const handle: RouteHandle = {
    breadcrumb: () => [{ content: <WebhookBreadcrumb /> }],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
    return [{ title: `Webhook • ${data?.webhook.name || data?.webhook.id}` }];
};
