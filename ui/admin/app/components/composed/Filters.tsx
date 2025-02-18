import { XIcon } from "lucide-react";
import { useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { $path, Routes } from "safe-routes";

import { Agent } from "~/lib/model/agents";
import { Task } from "~/lib/model/tasks";
import { User } from "~/lib/model/users";
import { RouteService } from "~/lib/service/routeService";

import { Button } from "~/components/ui/button";

type QueryParams = {
	agentId?: string;
	userId?: string;
	taskId?: string;
	createdStart?: string;
	createdEnd?: string;
};

export function Filters({
	agentMap,
	userMap,
	taskMap,
	url,
}: {
	agentMap?: Map<string, Agent>;
	userMap?: Map<string, User>;
	taskMap?: Map<string, Task>;
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

		const deleteFilters = (...params: (keyof QueryParams)[]) => {
			const newQuery = { ...query };
			params.forEach((param) => delete newQuery[param]);

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
					onRemove: () => deleteFilters("agentId"),
				},
			"userId" in filters &&
				filters.userId &&
				userMap && {
					key: "userId",
					label: "User",
					value: userMap.get(filters.userId)?.email ?? filters.userId,
					onRemove: () => deleteFilters("userId"),
				},
			"taskId" in filters &&
				filters.taskId &&
				taskMap && {
					key: "taskId",
					label: "Task",
					value: taskMap?.get(filters.taskId)?.name ?? filters.taskId,
					onRemove: () => deleteFilters("taskId"),
				},
			"createdStart" in filters &&
				filters.createdStart && {
					key: "createdStart",
					label: "Created",
					value: `${new Date(filters.createdStart).toLocaleDateString()} ${filters.createdEnd ? `- ${new Date(filters.createdEnd).toLocaleDateString()}` : ""}`,
					onRemove: () => deleteFilters("createdStart", "createdEnd"),
				},
		].filter((x) => !!x);
	}, [navigate, searchParams, agentMap, userMap, taskMap, url]);

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
