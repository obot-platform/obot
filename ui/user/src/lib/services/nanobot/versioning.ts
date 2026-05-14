export function hasNewerVersion(
	latestVersion?: number | string,
	currentInstalledVersion?: string
): boolean {
	const latest = latestVersion == null ? NaN : parseInt(String(latestVersion), 10);
	const current = currentInstalledVersion == null ? NaN : parseInt(currentInstalledVersion, 10);
	return Number.isFinite(latest) && Number.isFinite(current) && latest > current;
}
