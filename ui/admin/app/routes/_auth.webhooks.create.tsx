import { useNavigate } from "@remix-run/react";
import { $path } from "remix-routes";

import { WebhookForm } from "~/components/webhooks/WebhookForm";

export default function CreateWebhookPage() {
    const navigate = useNavigate();

    return <WebhookForm onSuccess={() => navigate($path("/webhooks"))} />;
}
