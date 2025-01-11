import { EyeIcon } from "lucide-react";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { UserService } from "~/lib/service/api/userService";

import { UserAuthorizationSelect } from "~/components/agent/UserAuthorizationSelect";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type AgentAccessControlProps = {
	agent: Agent;
};

export function AgentAccessControl({ agent }: AgentAccessControlProps) {
	const { data: users, isLoading: usersLoading } = useSWR(
		UserService.getUsers.key(),
		UserService.getUsers
	);

	return (
		<Dialog>
			<Tooltip>
				<TooltipTrigger asChild>
					<DialogTrigger asChild>
						<Button variant="ghost" size="icon">
							<EyeIcon />
						</Button>
					</DialogTrigger>
				</TooltipTrigger>
				<TooltipContent>Access Control</TooltipContent>
			</Tooltip>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Access Control</DialogTitle>
				</DialogHeader>

				<DialogDescription className="mb-4">
					<p>
						By default, admins will have access to <b>{agent.name}</b>.{" "}
					</p>

					<p className="mt-2 leading-tight">
						To extend its access to other users, add or include their email in
						the list below.
					</p>
				</DialogDescription>

				<div className="flex flex-col gap-2">
					{usersLoading ? (
						<LoadingSpinner className="m-l-2 text-muted-foreground" />
					) : (
						<UserAuthorizationSelect agentId={agent.id} users={users ?? []} />
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
}
