export type FileScannerConfigBase = {
	id: number;
	updatedAt: string;
	providerName: string;
};

export type FileScannerConfig = FileScannerConfigBase;

export type UpdateFileScannerConfig = FileScannerConfigBase;
