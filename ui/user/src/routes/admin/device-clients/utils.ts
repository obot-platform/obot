import type {
	DeviceScan,
	DeviceScanClient,
	DeviceScanMCPServer,
	DeviceScanSkill
} from '$lib/services/admin/types';

interface DeviceClient extends DeviceScanClient {
	id: string; // equivalent to name for now
	userIds: string[];
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

export const compileDeviceClients = (devices: DeviceScan[]) => {
	return devices?.reduce<Map<string, DeviceClient>>((acc, scan) => {
		const users = scanUserIds(scan);
		for (const client of scan.clients ?? []) {
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
					...existing,
					...client,
					id: key,
					userIds: uniqStrings([...existing.userIds, ...users]),
					skills: Object.values(skillByKey),
					mcpServers: Object.values(mcpByKey)
				});
			} else {
				acc.set(key, {
					...client,
					id: key,
					userIds: uniqStrings(users),
					skills: skillsForClient,
					mcpServers: mcpForClient
				});
			}
		}
		return acc;
	}, new Map<string, DeviceClient>());
};
