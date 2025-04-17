import { MetaFunction } from "react-router";

import { RouteHandle } from "~/lib/service/routeHandles";

import { FileScannerProviderList } from "~/components/auth-and-model-providers/FileScannerProviderLists";
import { useFileScannerProviders } from "~/hooks/file-scanner-providers/useFileScannerProviders";

export default function FileScannerProviders() {
	const { fileScannerProviders } = useFileScannerProviders();
	return (
		<div>
			<div className="relative px-8 pb-8">
				<div className="sticky top-0 z-10 flex flex-col gap-4 bg-background py-8">
					<div className="flex items-center justify-between">
						<h2 className="mb-0 pb-0">File Scanner Providers</h2>
					</div>
					<div className="h-16 w-full" />
				</div>

				<div className="flex h-full flex-col gap-8 overflow-hidden">
					<FileScannerProviderList
						fileScannerProviders={fileScannerProviders}
					/>
				</div>
			</div>
		</div>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "File Scanner Providers" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • File Scanner Providers` }];
};
