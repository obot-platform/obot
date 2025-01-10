import { CircleCheckIcon, CircleSlashIcon } from "lucide-react";
import { Link } from "react-router";

import { ModelProvider } from "~/lib/model/modelProviders";

import { ModelProviderConfigure } from "~/components/model-providers/ModelProviderConfigure";
import { ModelProviderMenu } from "~/components/model-providers/ModelProviderDropdown";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { ModelProvidersModels } from "~/components/model-providers/ModelProviderModels";
import {
	ModelProviderLinks,
	RecommendedModelProviders,
} from "~/components/model-providers/constants";
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
									<ModelProviderMenu modelProvider={modelProvider} />
								</div>
							) : (
								<div className="h-9 w-9" />
							)}
						</CardHeader>
						<CardContent className="flex flex-col items-center gap-4">
							<Link to={ModelProviderLinks[modelProvider.id]}>
								<ModelProviderIcon modelProvider={modelProvider} size="lg" />
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
							<ModelProviderConfigure modelProvider={modelProvider} />
						</CardContent>
					</Card>
				))}
			</div>
		</div>
	);
}
