import {
	FileScannerConfig,
	UpdateFileScannerConfig,
} from "~/lib/model/fileScannerConfig";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getConfig() {
	const { data } = await request<FileScannerConfig>({
		url: ApiRoutes.fileScannerConfig.getFileScannerConfig().url,
	});

	return data;
}
getConfig.key = () => ({
	url: ApiRoutes.fileScannerConfig.getFileScannerConfig().url,
});

async function updateConfig(config: UpdateFileScannerConfig) {
	const { data } = await request<FileScannerConfig>({
		url: ApiRoutes.fileScannerConfig.updateFileScannerConfig().url,
		method: "PUT",
		data: config,
	});

	return data;
}

export const FileScannerConfigApiService = {
	getConfig,
	updateConfig,
};
