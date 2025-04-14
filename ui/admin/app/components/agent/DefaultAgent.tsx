import { useEffect, useState } from "react";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";

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
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";

export function DefaultAgent({
	agents,
	disabled,
}: {
	agents: Agent[];
	disabled?: boolean;
}) {
	const [open, setOpen] = useState(false);
	const [defaultAgent, setDefaultAgent] = useState<Agent | null>(null);

	useEffect(() => {
		setDefaultAgent(agents.find((agent) => agent.default) ?? null);
	}, [agents]);

	const asyncSetDefaultAgent = useAsync(AgentService.setDefaultAgent, {
		onSuccess: () => {
			AgentService.getAgents.revalidate();
			setOpen(false);
		},
	});

	function handleDefaultAgentChange(agentId: string) {
		const agent = agents.find((agent) => agent.id === agentId);
		if (agent) {
			asyncSetDefaultAgent.executeAsync({ agentId });
			setDefaultAgent(agent);
		}
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button disabled={disabled}>Set Default Agent</Button>
			</DialogTrigger>

			<DialogContent className="w-full max-w-md">
				<DialogHeader>
					<DialogTitle>Default Agent</DialogTitle>
				</DialogHeader>

				<DialogDescription>
					When no agent is specified during Obot creation, this agent will be
					the default.
				</DialogDescription>

				<Select
					onValueChange={handleDefaultAgentChange}
					value={defaultAgent?.id}
				>
					<SelectTrigger>
						<SelectValue placeholder="Select Default Agent..." />
					</SelectTrigger>

					<SelectContent position="item-aligned">
						{agents.map((agent) => (
							<SelectItem key={agent.id} value={agent.id}>
								{agent.name}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</DialogContent>
		</Dialog>
	);
}
