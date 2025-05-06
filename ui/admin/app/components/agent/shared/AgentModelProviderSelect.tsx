import { useEffect, useState } from "react";
import useSWR from "swr";

import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";
import { cn } from "~/lib/utils";

import { useTheme } from "~/components/theme";
import { Card, CardContent } from "~/components/ui/card";
import { Checkbox } from "~/components/ui/checkbox";
import { Label } from "~/components/ui/label";

type AgentModelProviderSelectProps = {
	entity: { allowedModelProviders?: string[] };
	onChange: (value: { allowedModelProviders?: string[] }) => void;
};

export function AgentModelProviderSelect({
	entity,
	onChange,
}: AgentModelProviderSelectProps) {
	const { data: modelProviders = [] } = useSWR(
		ModelProviderApiService.getModelProviders.key(),
		() => ModelProviderApiService.getModelProviders()
	);
	const { isDark } = useTheme();

	const [selectedProviders, setSelectedProviders] = useState<string[]>(
		entity.allowedModelProviders || []
	);

	useEffect(() => {
		setSelectedProviders(entity.allowedModelProviders || []);
	}, [entity.allowedModelProviders]);

	const handleProviderChange = (providerId: string, checked: boolean) => {
		const newSelectedProviders = checked
			? [...selectedProviders, providerId]
			: selectedProviders.filter((id) => id !== providerId);

		setSelectedProviders(newSelectedProviders);
		onChange({ allowedModelProviders: newSelectedProviders });
	};

	return (
		<div className="space-y-4">
			<div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
				{modelProviders.map((provider) => (
					<Card key={provider.id} className="overflow-hidden">
						<CardContent className="pt-6">
							<div className="flex items-center space-x-2">
								<Checkbox
									id={`provider-${provider.id}`}
									checked={selectedProviders.includes(provider.id)}
									onCheckedChange={(checked) =>
										handleProviderChange(provider.id, checked as boolean)
									}
								/>
								<div className="flex items-center gap-2">
									{(provider.icon || provider.iconDark) && (
										<img
											src={
												isDark && provider.iconDark
													? provider.iconDark
													: provider.icon
											}
											alt={provider.name}
											className={cn("h-5 w-5", {
												"dark:invert": isDark && !provider.iconDark,
											})}
										/>
									)}
									<Label
										htmlFor={`provider-${provider.id}`}
										className="cursor-pointer"
									>
										{provider.name}
									</Label>
								</div>
							</div>
						</CardContent>
					</Card>
				))}
			</div>
		</div>
	);
}
