import { PenIcon } from "lucide-react";
import { toast } from "sonner";

import { Agent } from "~/lib/model/agents";
import { EnvVariable } from "~/lib/model/environmentVariables";
import { EnvironmentApiService } from "~/lib/service/api/EnvironmentApiService";

import { EnvForm } from "~/components/agent/shared/AgentEnvironmentVariableForm";
import { SelectList } from "~/components/composed/SelectModule";
import { Button } from "~/components/ui/button";
import { Card, CardDescription } from "~/components/ui/card";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { useAsync } from "~/hooks/useAsync";

type EnvironmentVariableSectionProps = {
	entity: Agent;
	entityType: "agent";
	onUpdate: (env: Partial<Agent>) => void;
};

export function EnvironmentVariableSection({
	entity,
	entityType,
	onUpdate,
}: EnvironmentVariableSectionProps) {
	const revealEnv = useAsync(EnvironmentApiService.getEnvVariables);

	const onOpenChange = (open: boolean) => {
		if (open) {
			revealEnv.execute(entity.id);
		} else {
			revealEnv.clear();
		}
	};

	const updateEnv = useAsync(EnvironmentApiService.updateEnvVariables, {
		onSuccess: (_, params) => {
			toast.success("Environment variables updated");
			revealEnv.clear();

			onUpdate({
				env: Object.keys(params[1]).map((name) => ({
					name,
					value: "",
				})),
			});
		},
	});

	const open = !!revealEnv.data;

	const items = entity.env ?? [];

	return (
		<div className="space-y-4">
			<h4>Environment Variables</h4>

			<CardDescription>
				Add environment variables that will be available to all tools as key
				value pairs.
			</CardDescription>

			<div className="flex flex-col gap-2">
				{!!items.length && (
					<Card className="px-4 py-2">
						<SelectList
							getItemKey={(item) => item.name}
							items={items}
							renderItem={renderItem}
							selected={items.map((item) => item.name)}
						/>
					</Card>
				)}

				<Dialog open={open} onOpenChange={onOpenChange}>
					<DialogTrigger asChild>
						<Button
							variant="ghost"
							loading={revealEnv.isLoading}
							className="self-end"
							startContent={<PenIcon />}
						>
							Environment Variables
						</Button>
					</DialogTrigger>

					<DialogContent className="max-w-3xl">
						<DialogHeader>
							<DialogTitle>Environment Variables</DialogTitle>
						</DialogHeader>

						<DialogDescription>
							Environment variables are used to store values that can be used in
							your {entityType}.
						</DialogDescription>

						{revealEnv.data && (
							<EnvForm
								defaultValues={revealEnv.data}
								isLoading={updateEnv.isLoading}
								onSubmit={(values) => updateEnv.execute(entity.id, values)}
							/>
						)}
					</DialogContent>
				</Dialog>
			</div>
		</div>
	);

	function renderItem(item: EnvVariable) {
		return (
			<div className="flex w-full items-center justify-between gap-2">
				<p className="flex-1">{item.name}</p>
				<p>{"•".repeat(15)}</p>
			</div>
		);
	}
}
