import { useNavigate } from "@remix-run/react";
import { $path } from "remix-routes";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { WebhookForm } from "~/components/webhooks/WebhookForm";
import { useAsync } from "~/hooks/useAsync";

export default function CreateWebhookPage() {
    const navigate = useNavigate();

    const createWebhook = useAsync(WebhookApiService.createWebhook, {
        onSuccess: () => navigate($path("/webhooks")),
    });

    return <WebhookForm onSubmit={createWebhook.execute} />;
}
