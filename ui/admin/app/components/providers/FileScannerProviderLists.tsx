import { CircleCheckIcon, CircleSlashIcon } from "lucide-react";
import { Link } from "react-router";

import { FileScannerProvider } from "~/lib/model/providers";

import { ProviderConfigure } from "~/components/providers/ProviderConfigure";
import { ProviderIcon } from "~/components/providers/ProviderIcon";
import { ProviderMenu } from "~/components/providers/ProviderMenu";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent, CardHeader } from "~/components/ui/card";

export function FileScannerProviderList({
	fileScannerProviders,
}: {
	fileScannerProviders: FileScannerProvider[];
}) {
	return (
		<div className="space-y-4">
			<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
				{fileScannerProviders.map((fileScannerProvider) => (
					<Card key={fileScannerProvider.id}>
						<CardHeader className="flex flex-row items-center justify-end pb-4 pt-2">
							{fileScannerProvider.configured ? (
								<div className="flex flex-row items-center gap-2">
									<ProviderMenu provider={fileScannerProvider} />
								</div>
							) : (
								<div className="h-9 w-9" />
							)}
						</CardHeader>
						<CardContent className="flex flex-col items-center gap-4">
							<Link to={fileScannerProvider.link ?? ""}>
								<ProviderIcon provider={fileScannerProvider} size="lg" />
							</Link>
							<div className="text-center text-lg font-semibold">
								{fileScannerProvider.name}
							</div>

							<Badge className="pointer-events-none" variant="outline">
								{fileScannerProvider.configured ? (
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
							<ProviderConfigure provider={fileScannerProvider} />
						</CardContent>
					</Card>
				))}
			</div>
		</div>
	);
}
