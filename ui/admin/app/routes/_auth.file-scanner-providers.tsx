import { MetaFunction } from "react-router";

import { RouteHandle } from "~/lib/service/routeHandles";

import { FileScannerProviderList } from "~/components/providers/FileScannerProviderLists";
import { FileScannerConfigForm } from "~/components/providers/FileScannerSelectionForm";
import { useFileScannerProviders } from "~/hooks/file-scanner-providers/useFileScannerProviders";

export default function FileScannerProviders() {
	const { fileScannerProviders } = useFileScannerProviders();
	return (
		<div>
			<div className="relative px-8 pb-8">
				<div className="sticky top-0 z-10 flex flex-col gap-4 bg-background py-8">
					<div className="flex items-center justify-between">
						<h2 className="mb-0 pb-0">File Scanners</h2>
						<FileScannerConfigForm />
					</div>
					<p>
						Enabling a file scanner will enforce virus scanning on all user
						uploaded files.
					</p>
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
	breadcrumb: () => [{ content: "File Scanners" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • File Scanners` }];
};
