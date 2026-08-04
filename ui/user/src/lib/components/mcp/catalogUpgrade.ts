import type { CatalogUpgradePlan, MCPSubField } from '$lib/services';
import type { LaunchFormData } from './CatalogConfigureForm.svelte';

export function getCatalogUpgradeBlockers(plan: CatalogUpgradePlan): string[] {
	return [...(plan.validationFailures ?? [])];
}

export function catalogUpgradeNeedsConfiguration(plan: CatalogUpgradePlan): boolean {
	return (
		(plan.missingRequiredEnvVars?.length ?? 0) > 0 ||
		(plan.missingRequiredHeaders?.length ?? 0) > 0 ||
		Boolean(plan.missingURL)
	);
}

export function catalogUpgradeForm(plan: CatalogUpgradePlan): LaunchFormData {
	const missingEnv = new Set(plan.missingRequiredEnvVars ?? []);
	const missingHeaders = new Set(plan.missingRequiredHeaders ?? []);
	return {
		envs: plan.targetManifest.env?.filter((field) => missingEnv.has(field.key)).map(emptyField),
		headers: plan.targetManifest.remoteConfig?.headers
			?.filter((field) => missingHeaders.has(field.key))
			.map(emptyField),
		url: plan.targetManifest.remoteConfig?.url,
		urlRequired: Boolean(plan.missingURL)
	};
}

function emptyField(field: MCPSubField): MCPSubField & { value: string } {
	return { ...field, value: '' };
}

export function catalogUpgradeConfiguration(form?: LaunchFormData): Record<string, string> {
	const configuration: Record<string, string> = {};
	for (const field of [...(form?.envs ?? []), ...(form?.headers ?? [])]) {
		if (field.value) configuration[field.key] = field.value;
	}
	return configuration;
}

export function catalogUpgradeConfigurationComplete(
	plan: CatalogUpgradePlan,
	configuration: Record<string, string>,
	url?: string
): boolean {
	return (
		[...(plan.missingRequiredEnvVars ?? []), ...(plan.missingRequiredHeaders ?? [])].every((key) =>
			Boolean(configuration[key])
		) &&
		(!plan.missingURL || Boolean(url?.trim()))
	);
}
