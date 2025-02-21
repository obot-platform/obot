import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	redirect,
	useLoaderData,
	useMatch,
} from "react-router";
import useSWR, { preload } from "swr";

import { AgentService } from "~/lib/service/api/agentService";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { cn } from "~/lib/utils";

import { Agent } from "~/components/agent";
import { AgentProvider } from "~/components/agent/AgentContext";
import { AgentMeta } from "~/components/agent/AgentMeta";
import {
	ResizableHandle,
	ResizablePanel,
	ResizablePanelGroup,
} from "~/components/ui/resizable";
import { ScrollArea } from "~/components/ui/scroll-area";

export type SearchParams = RouteQueryParams<"agentSchema">;

export const clientLoader = async ({
	params,
	request,
}: ClientLoaderFunctionArgs) => {
	const url = new URL(request.url);

	const routeInfo = RouteService.getRouteInfo("/agents/:id", url, params);

	const { id: agentId } = routeInfo.pathParams;
	const { from } = routeInfo.query ?? {};

	if (!agentId) {
		throw redirect("/agents");
	}

	// preload the agent and default model aliases
	const response = await Promise.all([
		preload(
			DefaultModelAliasApiService.getAliases.key(),
			DefaultModelAliasApiService.getAliases
		),
		preload(...AgentService.getAgentById.swr({ agentId })),
	]);

	const agent = response[1];

	if (!agent) {
		throw redirect("/agents");
	}
	return { agent, from };
};

export default function ChatAgent() {
	const { agent } = useLoaderData<typeof clientLoader>();

	return (
		<ResizablePanelGroup direction="horizontal" className="flex-auto">
			<ResizablePanel defaultSize={75} minSize={25}>
				<ScrollArea className="h-full" enableScrollStick="bottom">
					<div className={cn("relative mx-auto flex h-full flex-col")}>
						<AgentProvider agent={agent}>
							<Agent key={agent.id} />
						</AgentProvider>
					</div>
				</ScrollArea>
			</ResizablePanel>
			<ResizableHandle />
			<ResizablePanel defaultSize={25} minSize={25}>
				<ScrollArea className="h-full">
					<AgentMeta agent={agent} />
				</ScrollArea>
			</ResizablePanel>
		</ResizablePanelGroup>
	);
}

const AgentBreadcrumb = () => {
	const match = useMatch("/agents/:id");

	const { data: agent } = useSWR(
		...AgentService.getAgentById.swr({ agentId: match?.params.id })
	);

	return <>{agent?.name || "New Agent"}</>;
};

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: <AgentBreadcrumb /> }],
};

export const meta: MetaFunction<typeof clientLoader> = ({ data }) => {
	return [{ title: `Agent • ${data?.agent?.name}` }];
};
