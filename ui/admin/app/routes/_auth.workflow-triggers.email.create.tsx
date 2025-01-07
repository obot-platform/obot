import { MetaFunction } from "react-router";

import { RouteHandle } from "~/lib/service/routeHandles";

import { EmailReceiverForm } from "~/components/workflow-triggers/EmailReceiverForm";

export default function CreateEmailReceiverPage() {
    return <EmailReceiverForm />;
}

export const handle: RouteHandle = {
    breadcrumb: () => [{ content: "Create Email Receiver" }],
};

export const meta: MetaFunction = () => {
    return [{ title: "Create Email Receiver" }];
};
