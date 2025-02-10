import { XIcon } from "lucide-react";
import { useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { $path, Routes } from "safe-routes";

import { Agent } from "~/lib/model/agents";
import { User } from "~/lib/model/users";
import { Workflow } from "~/lib/model/workflows";
import { RouteService } from "~/lib/service/routeService";

import { Button } from "~/components/ui/button";

type QueryParams = {
	agentId?: string;
	userId?: string;
	taskId?: string;
};

export function Filters({
	agentMap,
	userMap,
	workflowMap,
	url,
}: {
	agentMap?: Map<string, Agent>;
	userMap?: Map<string, User>;
	workflowMap?: Map<string, Workflow>;
	url: keyof Routes;
}) {
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	const filters = useMemo(() => {
		const query =
			(RouteService.getQueryParams(
				url,
				searchParams.toString()
			) as QueryParams) ?? {};
		const { ...filters } = query; // TODO: from

		const updateFilters = (param: keyof QueryParams) => {
			const newQuery = { ...query };
			delete newQuery[param];
			// Filter out null/undefined values and ensure all values are strings
			const cleanQuery = Object.fromEntries(
				Object.entries(newQuery)
					.filter(([_, v]) => v != null)
					.map(([k, v]) => [k, String(v)])
			) as Parameters<typeof $path>[1];
			return navigate($path(url, cleanQuery));
		};

		return [
			"agentId" in filters &&
				filters.agentId &&
				agentMap && {
					key: "agentId",
					label: "Agent",
					value: agentMap.get(filters.agentId)?.name ?? filters.agentId,
					onRemove: () => updateFilters("agentId"),
				},
			"userId" in filters &&
				filters.userId &&
				userMap && {
					key: "userId",
					label: "User",
					value: userMap.get(filters.userId)?.email ?? filters.userId,
					onRemove: () => updateFilters("userId"),
				},
			"taskId" in filters &&
				filters.taskId &&
				workflowMap && {
					key: "taskId",
					label: "Task",
					value: workflowMap?.get(filters.taskId)?.name ?? filters.taskId,
					onRemove: () => updateFilters("taskId"),
				},
		].filter((x) => !!x);
	}, [navigate, searchParams, agentMap, userMap, workflowMap, url]);

	return (
		<div className="flex gap-2 pb-2">
			{filters.map((filter) => (
				<Button
					key={filter.key}
					size="badge"
					onClick={filter.onRemove}
					variant="accent"
					shape="pill"
					endContent={<XIcon />}
				>
					<b>{filter.label}:</b> {filter.value}
				</Button>
			))}
		</div>
	);
}
