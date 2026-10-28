// Gateway profiles are model access policies: the users and groups they apply to, the models
// those users may use, and the budgets and guardrail policies that apply to them.
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import { matchesModelPattern } from '$lib/services/admin/utils';
import type { DefaultModelAlias, Model } from '$lib/services/user/types';

export const PROFILES_URL = '/models?view=profiles';

export function profileUrl(id: string): string {
	return `/models/profiles/${encodeURIComponent(id)}`;
}

// The page of the profile a budget or guardrail policy belongs to, or the profile list.
export function backToProfile(profileID: string | undefined | null): string {
	return profileID ? profileUrl(profileID) : PROFILES_URL;
}

// Default model aliases a profile can grant, as the backend allows.
const GRANTABLE_ALIASES = ['llm', 'llm-mini'];

// The models among `models` that a profile grants, resolved as the backend resolves them:
// only LLM models can be granted; "*" grants all of them, a trailing "*" matches target models,
// and an "obot://" reference grants the model its default alias points to. A budget covers
// only its profile's models.
export function profileGrantedModels(
	profile: Pick<ModelAccessPolicy, 'models'> | undefined,
	models: Model[],
	aliases: DefaultModelAlias[] = []
): Model[] {
	const eligible = models.filter((model) => model.usage === 'llm');
	const granted = new Set<string>();
	for (const { id } of profile?.models ?? []) {
		if (id === '*') {
			return eligible;
		}
		if (id.startsWith('obot://')) {
			const alias = id.slice('obot://'.length);
			const target = GRANTABLE_ALIASES.includes(alias)
				? aliases.find((a) => a.alias === alias)?.model
				: undefined;
			if (target) granted.add(target);
		} else if (id.endsWith('*')) {
			const prefix = id.slice(0, -1);
			for (const model of eligible) {
				if (matchesModelPattern(prefix, model.targetModel)) granted.add(model.id);
			}
		} else {
			granted.add(id);
		}
	}
	return eligible.filter((model) => granted.has(model.id));
}
