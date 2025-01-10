import { MetaFunction } from "react-router";

import { RouteHandle } from "~/lib/service/routeHandles";

import { WebhookForm } from "~/components/webhooks/WebhookForm";

export default function CreateWebhookPage() {
	return <WebhookForm />;
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Create Webhook" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Create Webhook` }];
};
