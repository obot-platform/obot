import type { ImagePullSecret } from './types';

export function canTest(secret: ImagePullSecret) {
	return !secret.manifest.enabled || !secret.status?.lastError;
}

// Reports whether a wildcard suffix pattern's prefix matches a model's target model, as the
// backend does (case-sensitive). LLM gateway targets are provider-qualified
// ("anthropic/claude-haiku-4-5"); a prefix without a provider, such as "claude-haiku-4-5",
// matches the part after the provider too.
export function matchesModelPattern(prefix: string, targetModel: string | undefined) {
	const target = targetModel || '';
	if (target.startsWith(prefix)) return true;
	const slash = target.indexOf('/');
	return slash >= 0 && !prefix.includes('/') && target.slice(slash + 1).startsWith(prefix);
}
