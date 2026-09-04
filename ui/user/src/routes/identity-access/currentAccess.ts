import { AdminService, type AccessControlRule, type AccessControlRuleSubject } from '$lib/services';

export type CurrentAccessKind = 'user' | 'group';

export type AccessMatchReason = 'everyone' | 'direct-user' | 'direct-group' | 'via-group';

export interface CurrentAccessTarget {
	kind: CurrentAccessKind;
	id: string;
	name: string;
	groupIds?: string[];
}

export type AccessResourceType =
	| 'mcpServerCatalogEntry'
	| 'mcpServer'
	| 'model'
	| 'skill'
	| 'skillRepository'
	| 'hostedAgent'
	| 'selector';

export interface AccessPolicyResource {
	type: AccessResourceType;
	id: string;
}

export interface MatchedAccessPolicy {
	id: string;
	displayName: string;
	href: `/${string}`;
	reasons: AccessMatchReason[];
	resources: AccessPolicyResource[];
	powerUserID?: string;
}

export interface McpAccessPolicyGroup {
	key: string;
	powerUserID?: string;
	policies: MatchedAccessPolicy[];
}

export interface MatchedAccessResource {
	key: string;
	id: string;
	type: AccessResourceType;
	name: string;
	typeLabel?: string;
	/** Policies that grant this resource to the target. */
	policies: MatchedAccessPolicy[];
}

export interface AccessResourceDescription {
	name: string;
	typeLabel?: string;
}

/** Resource id that grants access to every resource of a policy's kind. */
export const EVERYTHING_RESOURCE_ID = '*';

export interface CurrentAccessSections {
	mcp: MatchedAccessPolicy[];
	models: MatchedAccessPolicy[];
	skills: MatchedAccessPolicy[];
	hostedAgents: MatchedAccessPolicy[];
	vmcps: MatchedAccessPolicy[];
	profiles: MatchedAccessPolicy[];
}

export const ACCESS_MATCH_REASON_LABEL: Record<AccessMatchReason, string> = {
	everyone: 'All Obot Users',
	'direct-user': 'Assigned to this user',
	'direct-group': 'Assigned to this group',
	'via-group': 'Via group membership'
};

export function isEveryoneSubject(subject: AccessControlRuleSubject): boolean {
	return subject.id === '*';
}

export function subjectsApplyTo(
	subjects: AccessControlRuleSubject[] | undefined,
	target: CurrentAccessTarget
): AccessMatchReason[] {
	if (!subjects?.length) {
		return [];
	}

	const reasons = new Set<AccessMatchReason>();
	const groupIds = new Set(target.groupIds ?? []);
	if (target.kind === 'group') {
		groupIds.add(target.id);
	}

	for (const subject of subjects) {
		if (isEveryoneSubject(subject)) {
			reasons.add('everyone');
			continue;
		}

		if (target.kind === 'user' && subject.type === 'user' && subject.id === target.id) {
			reasons.add('direct-user');
			continue;
		}

		if (subject.type === 'group' && groupIds.has(subject.id)) {
			reasons.add(target.kind === 'group' ? 'direct-group' : 'via-group');
		}
	}

	return [...reasons];
}

export function mcpAccessPolicyHref(rule: AccessControlRule): `/${string}` {
	if (rule.powerUserWorkspaceID) {
		return `/mcp-servers/access-policies/w/${rule.powerUserWorkspaceID}/r/${rule.id}`;
	}
	return `/mcp-servers/access-policies/${rule.id}`;
}

function settledItems<T>(result: PromiseSettledResult<T[]>): T[] {
	return result.status === 'fulfilled' ? result.value : [];
}

function matchPolicies<
	T extends { id: string; displayName: string; subjects?: AccessControlRuleSubject[] }
>(
	policies: T[],
	target: CurrentAccessTarget,
	href: (policy: T) => `/${string}`,
	resources: (policy: T) => AccessPolicyResource[],
	additionalFields?: (policy: T) => Partial<MatchedAccessPolicy>
): MatchedAccessPolicy[] {
	return policies
		.flatMap((policy) => {
			const reasons = subjectsApplyTo(policy.subjects, target);
			if (reasons.length === 0) {
				return [];
			}
			return [
				{
					id: policy.id,
					displayName: policy.displayName,
					href: href(policy),
					reasons,
					resources: resources(policy),
					...additionalFields?.(policy)
				}
			];
		})
		.sort((a, b) => a.displayName.localeCompare(b.displayName));
}

/**
 * MCP wildcard selectors apply only within their registry. Keep global policies separate from
 * each power user's policies so one registry's wildcard does not hide another registry's grants.
 */
export function groupMcpAccessPolicies(policies: MatchedAccessPolicy[]): McpAccessPolicyGroup[] {
	const groups = new Map<string, McpAccessPolicyGroup>();

	for (const policy of policies) {
		const key = policy.powerUserID ?? 'global';
		const group = groups.get(key);
		if (group) {
			group.policies.push(policy);
		} else {
			groups.set(key, {
				key,
				powerUserID: policy.powerUserID,
				policies: [policy]
			});
		}
	}

	return [...groups.values()].sort((a, b) => {
		if (!a.powerUserID) return -1;
		if (!b.powerUserID) return 1;
		return a.powerUserID.localeCompare(b.powerUserID);
	});
}

export function grantsEverything(policies: MatchedAccessPolicy[]): boolean {
	return policies.some((policy) =>
		policy.resources.some((resource) => resource.id === EVERYTHING_RESOURCE_ID)
	);
}

/**
 * Flattens the resources granted by the matched policies into a deduplicated list, tracking which
 * policies granted each resource. `describe` turns a resource reference into display text.
 */
export function collectAccessResources(
	policies: MatchedAccessPolicy[],
	describe: (resource: AccessPolicyResource) => AccessResourceDescription
): MatchedAccessResource[] {
	const byKey = new Map<string, MatchedAccessResource>();

	for (const policy of policies) {
		for (const resource of policy.resources) {
			if (resource.id === EVERYTHING_RESOURCE_ID) {
				continue;
			}

			const key = `${resource.type}:${resource.id}`;
			const existing = byKey.get(key);
			if (existing) {
				if (!existing.policies.some((granting) => granting.id === policy.id)) {
					existing.policies.push(policy);
				}
				continue;
			}

			byKey.set(key, {
				key,
				id: resource.id,
				type: resource.type,
				...describe(resource),
				policies: [policy]
			});
		}
	}

	return [...byKey.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function dedupeById<T extends { id: string }>(items: T[]): T[] {
	const seen = new Set<string>();
	return items.filter((item) => {
		if (seen.has(item.id)) {
			return false;
		}
		seen.add(item.id);
		return true;
	});
}

export async function loadCurrentAccess(
	target: CurrentAccessTarget
): Promise<CurrentAccessSections> {
	const [mcpCatalog, mcpWorkspaces, models, skills, hostedAgents] = await Promise.allSettled([
		AdminService.listAccessControlRules(),
		AdminService.listAllUserWorkspaceAccessControlRules(),
		AdminService.listModelAccessPolicies(),
		AdminService.listSkillAccessPolicies(),
		AdminService.listHostedAgentAccessPolicies()
	]);

	return {
		mcp: matchPolicies(
			dedupeById([...settledItems(mcpCatalog), ...settledItems(mcpWorkspaces)]),
			target,
			mcpAccessPolicyHref,
			(policy) => policy.resources ?? [],
			(policy) => ({ powerUserID: policy.powerUserID })
		),
		models: matchPolicies(
			settledItems(models),
			target,
			(policy) => `/models/access-policies/${policy.id}`,
			(policy) =>
				(policy.models ?? []).map(({ id }) => ({
					type: id === EVERYTHING_RESOURCE_ID ? 'selector' : 'model',
					id
				}))
		),
		skills: matchPolicies(
			settledItems(skills),
			target,
			(policy) => `/skills/access-policies/${policy.id}`,
			(policy) => policy.resources ?? []
		),
		hostedAgents: matchPolicies(
			settledItems(hostedAgents),
			target,
			(policy) => `/hosted-agents/access-policies/${policy.id}`,
			(policy) => policy.resources ?? []
		),
		// TODO: access policies for vMCPs and profiles
		vmcps: [],
		profiles: []
	};
}

export function hasAnyCurrentAccess(sections: CurrentAccessSections): boolean {
	return (
		sections.mcp.length > 0 ||
		sections.models.length > 0 ||
		sections.skills.length > 0 ||
		sections.hostedAgents.length > 0
	);
}
