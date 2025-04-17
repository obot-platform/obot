import useSWR from "swr";

import { FileScannerProviderApiService } from "~/lib/service/api/fileScannerProviderApiService";

export function useFileScannerProviders() {
	const { data: fileScannerProviders, ...rest } = useSWR(
		FileScannerProviderApiService.getFileScannerProviders.key(),
		() => FileScannerProviderApiService.getFileScannerProviders(),
		{ fallbackData: [] }
	);
	return { fileScannerProviders: fileScannerProviders, ...rest };
}
