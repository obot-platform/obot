import { MetaFunction, useNavigate } from "react-router";
import { $path } from "safe-routes";

import { RouteHandle } from "~/lib/service/routeHandles";

import { EmailReceiverForm } from "~/components/workflow-triggers/EmailReceiverForm";

export default function CreateEmailReceiverPage() {
	const navigate = useNavigate();

	return (
		<EmailReceiverForm
			onContinue={() => navigate($path("/workflow-triggers"))}
		/>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Create Email Receiver" }],
};

export const meta: MetaFunction = () => {
	return [{ title: "Create Email Receiver" }];
};
