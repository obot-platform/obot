import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
	useMatch,
	useNavigate,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";

import { EmailReceiverForm } from "~/components/workflow-triggers/EmailReceiverForm";

export async function clientLoader({
	request,
	params,
}: ClientLoaderFunctionArgs) {
	const { pathParams } = RouteService.getRouteInfo(
		"/workflow-triggers/email/:receiver",
		new URL(request.url),
		params
	);

	await preload(
		EmailReceiverApiService.getEmailReceiverById.key(pathParams.receiver),
		() => EmailReceiverApiService.getEmailReceiverById(pathParams.receiver)
	);

	return { emailReceiverId: pathParams.receiver };
}

export default function EmailReceiverPage() {
	const { emailReceiverId } = useLoaderData<typeof clientLoader>();

	const navigate = useNavigate();
	const { data: emailReceiver } = useSWR(
		EmailReceiverApiService.getEmailReceiverById.key(emailReceiverId),
		({ emailReceiverId }) =>
			EmailReceiverApiService.getEmailReceiverById(emailReceiverId)
	);

	return (
		<EmailReceiverForm
			emailReceiver={emailReceiver}
			onContinue={() => navigate($path("/workflow-triggers"))}
		/>
	);
}

const EmailReceiverBreadcrumb = () => {
	const match = useMatch("/workflow-triggers/email/:receiver");

	return match?.params?.receiver || "Edit";
};

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: <EmailReceiverBreadcrumb /> }],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
	return [{ title: `Email Receiver • ${data?.emailReceiverId}` }];
};
