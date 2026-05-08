import type {
	DeviceScan,
	DeviceScanClient,
	DeviceScanMCPServer,
	DeviceScanSkill,
	OrgUser
} from '$lib/services/admin/types';

export type DeviceClientUser = OrgUser & {
	userConfigPath?: string;
	userInstallPath?: string;
};

// TODO: move to admin/types when /api/devices/client route is available
export interface DeviceClient {
	id: string; // equivalent to name for now
	name: string;
	users: DeviceClientUser[];
	skills: DeviceScanSkill[];
	mcpServers: DeviceScanMCPServer[];
}

function scanUserIds(scan: { submittedBy?: string; username?: string }): string[] {
	const ids: string[] = [];
	const sb = scan.submittedBy?.trim();
	const un = scan.username?.trim();
	if (sb) ids.push(sb);
	if (un) ids.push(un);
	return ids;
}

function skillDedupeKey(s: DeviceScanSkill): string {
	return [s.client, s.name, s.file ?? '', s.projectPath ?? ''].join('\0');
}

function mcpDedupeKey(m: DeviceScanMCPServer): string {
	return [m.client, m.name, m.transport, m.configHash ?? '', m.url ?? '', m.command ?? ''].join(
		'\0'
	);
}

function uniqStrings(xs: string[]): string[] {
	const seen: Record<string, true> = Object.create(null);
	const out: string[] = [];
	for (const x of xs) {
		if (!seen[x]) {
			seen[x] = true;
			out.push(x);
		}
	}
	return out;
}

/** Minimal OrgUser when a scan references an id not present in the org directory. */
function orgUserPlaceholder(id: string): OrgUser {
	return {
		id,
		username: id,
		email: '',
		created: '',
		explicitRole: false,
		role: 0,
		effectiveRole: 0,
		groups: [],
		iconURL: ''
	};
}

function resolveOrgUsers(ids: string[], orgUserById: Map<string, OrgUser>): OrgUser[] {
	return uniqStrings(ids).map((id) => orgUserById.get(id) ?? orgUserPlaceholder(id));
}

function augmentUsersWithClientPaths(
	users: OrgUser[],
	client: DeviceScanClient
): DeviceClientUser[] {
	return users.map((u) => ({
		...u,
		userInstallPath: client.installPath,
		userConfigPath: client.configPath
	}));
}

function mergeOrgUsers(
	existing: DeviceClientUser[],
	additions: DeviceClientUser[]
): DeviceClientUser[] {
	const byId = new Map<string, DeviceClientUser>();
	for (const u of existing) byId.set(u.id, u);
	for (const u of additions) {
		const prev = byId.get(u.id);
		if (!prev) {
			byId.set(u.id, u);
		} else {
			byId.set(u.id, {
				...prev,
				...u,
				userInstallPath: u.userInstallPath ?? prev.userInstallPath,
				userConfigPath: u.userConfigPath ?? prev.userConfigPath
			});
		}
	}
	return [...byId.values()];
}

export const compileDeviceClients = (devices: DeviceScan[], users: OrgUser[]) => {
	const orgUserById = new Map<string, OrgUser>(users.map((u) => [u.id, u]));
	return devices?.reduce<Map<string, DeviceClient>>((acc, scan) => {
		for (const client of scan.clients ?? []) {
			const scanUsers = augmentUsersWithClientPaths(
				resolveOrgUsers(scanUserIds(scan), orgUserById),
				client
			);
			const key = client.name.trim() || '__unnamed__';
			const skillsForClient = (scan.skills ?? []).filter((s) => s.client === client.name);
			const mcpForClient = (scan.mcpServers ?? []).filter((m) => m.client === client.name);

			const existing = acc.get(key);
			if (existing) {
				const skillByKey: Record<string, DeviceScanSkill> = Object.create(null);
				for (const s of existing.skills) skillByKey[skillDedupeKey(s)] = s;
				for (const s of skillsForClient) skillByKey[skillDedupeKey(s)] = s;

				const mcpByKey: Record<string, DeviceScanMCPServer> = Object.create(null);
				for (const m of existing.mcpServers) mcpByKey[mcpDedupeKey(m)] = m;
				for (const m of mcpForClient) mcpByKey[mcpDedupeKey(m)] = m;

				acc.set(key, {
					id: key,
					name: existing.name,
					users: mergeOrgUsers(existing.users, scanUsers),
					skills: Object.values(skillByKey),
					mcpServers: Object.values(mcpByKey)
				});
			} else {
				acc.set(key, {
					id: key,
					name: client.name,
					users: scanUsers,
					skills: skillsForClient,
					mcpServers: mcpForClient
				});
			}
		}
		return acc;
	}, new Map<string, DeviceClient>());
};
