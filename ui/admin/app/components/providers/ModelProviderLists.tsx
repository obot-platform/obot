import { CircleCheckIcon, CircleSlashIcon } from "lucide-react";
import { Link } from "react-router";

import { ModelProvider } from "~/lib/model/providers";

import { ModelProvidersModels } from "~/components/providers/ModelProviderModels";
import { ProviderConfigure } from "~/components/providers/ProviderConfigure";
import { ProviderIcon } from "~/components/providers/ProviderIcon";
import { ProviderMenu } from "~/components/providers/ProviderMenu";
import { RecommendedModelProviders } from "~/components/providers/constants";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent, CardHeader } from "~/components/ui/card";

export function ModelProviderList({
	modelProviders,
}: {
	modelProviders: ModelProvider[];
}) {
	return (
		<div className="space-y-4">
			<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
				{modelProviders.map((modelProvider) => (
					<Card key={modelProvider.id}>
						<CardHeader className="flex flex-row items-center justify-between pb-4 pt-2">
							{RecommendedModelProviders.includes(modelProvider.id) ? (
								<Badge variant="faded">Recommended</Badge>
							) : (
								<div />
							)}
							{modelProvider.configured ? (
								<div className="flex flex-row items-center gap-2">
									<ModelProvidersModels modelProvider={modelProvider} />
									<ProviderMenu provider={modelProvider} />
								</div>
							) : (
								<div className="h-9 w-9" />
							)}
						</CardHeader>
						<CardContent className="flex flex-col items-center gap-4">
							<Link to={modelProvider.link ?? ""}>
								<ProviderIcon provider={modelProvider} size="lg" />
							</Link>
							<div className="text-center text-lg font-semibold">
								{modelProvider.name}
							</div>

							<Badge className="pointer-events-none" variant="outline">
								{modelProvider.configured ? (
									<span className="flex items-center gap-1">
										<CircleCheckIcon size={18} className="text-success" />{" "}
										Configured
									</span>
								) : (
									<span className="flex items-center gap-1">
										<CircleSlashIcon size={18} className="text-destructive" />
										Not Configured
									</span>
								)}
							</Badge>
							<ProviderConfigure provider={modelProvider} />
						</CardContent>
					</Card>
				))}
			</div>
		</div>
	);
}
