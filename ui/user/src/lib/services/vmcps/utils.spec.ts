import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { SHORT_DESCRIPTION_MAX_LENGTH } from './constants';
import type { RectLike } from './types';
import {
	appendComponentLabel,
	borderAnchor,
	buildMcpServerFilterOptions,
	buildVMcpComponentFilterOptions,
	buildWirePath,
	distanceToRect,
	filterMcpServersByCategories,
	isJoinedComponentLabel,
	isWorkspaceOwned,
	joinComponentLabels,
	matchesQuery,
	sortMcpServers,
	sortVMcps
} from './utils';
import { describe, expect, it } from 'vitest';

function rect(left: number, top: number, width: number, height: number): RectLike {
	return { left, top, width, height, right: left + width, bottom: top + height };
}

function pointsOf(path: string) {
	return path.split(' ').map((command) => {
		const [x, y] = command.slice(1).split(',').map(Number);
		return { x, y };
	});
}

describe('isWorkspaceOwned', () => {
	it('is true for a workspace server', () => {
		expect(
			isWorkspaceOwned(
				createMCPCatalogEntry({ id: 'entry-1', name: 'Slack', powerUserWorkspaceID: 'ws-1' })
			)
		).toBe(true);
	});

	it('is true for a power-user server without a workspace id', () => {
		expect(
			isWorkspaceOwned(
				createMCPCatalogEntry({ id: 'entry-1', name: 'Slack', powerUserID: 'user-1' })
			)
		).toBe(true);
	});

	it('is false for a shared catalog server', () => {
		expect(isWorkspaceOwned(createMCPCatalogEntry({ id: 'entry-1', name: 'Slack' }))).toBe(false);
	});
});

describe('matchesQuery', () => {
	const entry = createMCPCatalogEntry({
		id: 'entry-1',
		name: 'GitHub',
		manifest: { shortDescription: 'Issues and pull requests', description: 'Source forge' }
	});

	it('matches the name regardless of case', () => {
		expect(matchesQuery(entry, 'github')).toBe(true);
		expect(matchesQuery(entry, 'GITHUB')).toBe(true);
	});

	it('matches the short description and the description', () => {
		expect(matchesQuery(entry, 'pull requests')).toBe(true);
		expect(matchesQuery(entry, 'forge')).toBe(true);
	});

	it('does not match unrelated text', () => {
		expect(matchesQuery(entry, 'slack')).toBe(false);
	});
});

describe('sortVMcps', () => {
	const zeta = createMCPCatalogEntry({
		id: 'vmcp-z',
		name: 'Zeta',
		runtime: 'composite',
		created: '2020-01-01T00:00:00.000Z'
	});
	const alpha = createMCPCatalogEntry({
		id: 'vmcp-a',
		name: 'Alpha',
		runtime: 'composite',
		created: '2024-06-01T00:00:00.000Z',
		manifest: { compositeConfig: { componentServers: [{ catalogEntryID: 'entry-1' }] } }
	});

	it('orders by name, ignoring case', () => {
		expect(sortVMcps([zeta, alpha], 'name').map((entry) => entry.id)).toEqual(['vmcp-a', 'vmcp-z']);
	});

	it('orders by created date, newest first', () => {
		expect(sortVMcps([zeta, alpha], 'created').map((entry) => entry.id)).toEqual([
			'vmcp-a',
			'vmcp-z'
		]);
	});

	it('orders by component server count, largest first', () => {
		expect(sortVMcps([zeta, alpha], 'componentServers').map((entry) => entry.id)).toEqual([
			'vmcp-a',
			'vmcp-z'
		]);
	});
});

describe('vMCP filter options', () => {
	const first = createMCPCatalogEntry({
		id: 'vmcp-a',
		name: 'Alpha',
		runtime: 'composite',
		powerUserID: 'user-1',
		manifest: { compositeConfig: { componentServers: [{ catalogEntryID: 'entry-github' }] } }
	});
	const second = createMCPCatalogEntry({
		id: 'vmcp-b',
		name: 'Bravo',
		runtime: 'composite',
		manifest: { compositeConfig: { componentServers: [{ catalogEntryID: 'entry-github' }] } }
	});

	it('lists component servers without duplicates', () => {
		expect(
			buildVMcpComponentFilterOptions([first, second], (id) =>
				id === 'entry-github' ? 'GitHub' : undefined
			)
		).toEqual([{ id: 'entry-github', label: 'GitHub' }]);
	});
});

describe('sortMcpServers', () => {
	const zeta = createMCPCatalogEntry({
		id: 'entry-z',
		name: 'Zeta',
		created: '2020-01-01T00:00:00.000Z'
	});
	const alpha = createMCPCatalogEntry({
		id: 'entry-a',
		name: 'Alpha',
		created: '2024-06-01T00:00:00.000Z'
	});

	it('orders alphabetically A-Z, ignoring case', () => {
		expect(sortMcpServers([zeta, alpha], 'nameAsc').map((entry) => entry.id)).toEqual([
			'entry-a',
			'entry-z'
		]);
	});

	it('orders alphabetically Z-A, ignoring case', () => {
		expect(sortMcpServers([zeta, alpha], 'nameDesc').map((entry) => entry.id)).toEqual([
			'entry-z',
			'entry-a'
		]);
	});

	it('orders by created date, newest first', () => {
		expect(sortMcpServers([zeta, alpha], 'created').map((entry) => entry.id)).toEqual([
			'entry-a',
			'entry-z'
		]);
	});

	it('orders by hardcoded catalog popularity, then name', () => {
		const gmail = createMCPCatalogEntry({ id: 'entry-gmail', name: 'Gmail' });
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack Workspace' });
		const obscure = createMCPCatalogEntry({ id: 'entry-obscure', name: 'Obscure Tool' });

		expect(sortMcpServers([obscure, slack, gmail], 'popularity').map((entry) => entry.id)).toEqual([
			'entry-gmail',
			'entry-slack',
			'entry-obscure'
		]);
	});
});

describe('filterMcpServersByCategories', () => {
	const github = createMCPCatalogEntry({
		id: 'entry-github',
		name: 'GitHub',
		manifest: { metadata: { categories: 'devtools,source-control' } }
	});
	const slack = createMCPCatalogEntry({
		id: 'entry-slack',
		name: 'Slack',
		manifest: { metadata: { categories: 'communication' } }
	});
	const uncategorized = createMCPCatalogEntry({ id: 'entry-plain', name: 'Plain' });

	it('returns every entry when no categories are selected', () => {
		expect(
			filterMcpServersByCategories([github, slack, uncategorized], '').map((entry) => entry.id)
		).toEqual(['entry-github', 'entry-slack', 'entry-plain']);
	});

	it('keeps servers that match any selected category', () => {
		expect(
			filterMcpServersByCategories([github, slack, uncategorized], 'devtools,communication').map(
				(entry) => entry.id
			)
		).toEqual(['entry-github', 'entry-slack']);
	});
});

describe('buildMcpServerFilterOptions', () => {
	it('lists unique categories in alphabetical order', () => {
		const github = createMCPCatalogEntry({
			id: 'entry-github',
			name: 'GitHub',
			manifest: { metadata: { categories: 'source-control,devtools' } }
		});
		const gitlab = createMCPCatalogEntry({
			id: 'entry-gitlab',
			name: 'GitLab',
			manifest: { metadata: { categories: 'devtools' } }
		});

		expect(buildMcpServerFilterOptions([github, gitlab])).toEqual([
			{ id: 'devtools', label: 'devtools' },
			{ id: 'source-control', label: 'source-control' }
		]);
	});
});

describe('distanceToRect', () => {
	it('is zero inside the rect', () => {
		expect(distanceToRect(rect(0, 0, 100, 100), { x: 50, y: 50 })).toBe(0);
	});

	it('measures to the nearest edge', () => {
		expect(distanceToRect(rect(0, 0, 100, 100), { x: 130, y: 50 })).toBe(30);
	});

	it('measures diagonally past a corner', () => {
		expect(distanceToRect(rect(0, 0, 100, 100), { x: 130, y: 140 })).toBeCloseTo(50);
	});
});

describe('borderAnchor', () => {
	it('lands on the right edge when pulled sideways', () => {
		expect(borderAnchor(rect(0, 0, 100, 100), { x: 400, y: 50 })).toEqual({ x: 100, y: 50 });
	});

	it('lands on the top edge when pulled upward', () => {
		expect(borderAnchor(rect(0, 0, 100, 100), { x: 50, y: -400 })).toEqual({ x: 50, y: 0 });
	});

	it('stays at the center when the point is the center', () => {
		expect(borderAnchor(rect(0, 0, 100, 100), { x: 50, y: 50 })).toEqual({ x: 50, y: 50 });
	});
});

describe('buildWirePath', () => {
	it('pins both ends to the anchors regardless of phase', () => {
		const from = { x: 100, y: 200 };
		const to = { x: 500, y: 320 };

		for (const phase of [0, 1.4, 3.9]) {
			const points = pointsOf(buildWirePath(from, to, phase));
			expect(points.at(0)).toEqual(from);
			expect(points.at(-1)).toEqual(to);
		}
	});

	it('bows away from the straight line at the midpoint', () => {
		const points = pointsOf(buildWirePath({ x: 0, y: 0 }, { x: 400, y: 0 }));
		const midpoint = points[Math.floor(points.length / 2)];

		expect(Math.abs(midpoint.y)).toBeGreaterThan(10);
	});

	it('collapses to a single move for coincident points', () => {
		expect(buildWirePath({ x: 10, y: 10 }, { x: 10, y: 10 })).toBe('M10.0,10.0');
	});
});

describe('joinComponentLabels', () => {
	it('joins trimmed titles with a comma', () => {
		expect(joinComponentLabels([' Slack ', '', 'GitHub'])).toBe('Slack, GitHub');
	});
});

describe('isJoinedComponentLabel', () => {
	it('matches a single component title', () => {
		expect(isJoinedComponentLabel('Slack', ['Slack'])).toBe(true);
	});

	it('matches a truncated description to the length cap', () => {
		const parts = ['a'.repeat(100), 'b'.repeat(100)];
		const truncated = joinComponentLabels(parts).slice(0, SHORT_DESCRIPTION_MAX_LENGTH);
		expect(isJoinedComponentLabel(truncated, parts, SHORT_DESCRIPTION_MAX_LENGTH)).toBe(true);
	});

	it('rejects a custom title', () => {
		expect(isJoinedComponentLabel('My Gateway', ['Slack'])).toBe(false);
	});
});

describe('appendComponentLabel', () => {
	it('appends when the current value is the joined existing titles', () => {
		expect(appendComponentLabel('Slack', ['Slack'], 'GitHub')).toBe('Slack, GitHub');
	});

	it('leaves a custom title unchanged', () => {
		expect(appendComponentLabel('Ops Gateway', ['Slack'], 'GitHub')).toBe('Ops Gateway');
	});

	it('truncates descriptions at 160 characters', () => {
		const existing = 'a'.repeat(100);
		const added = 'b'.repeat(100);
		const next = appendComponentLabel(existing, [existing], added, SHORT_DESCRIPTION_MAX_LENGTH);
		expect(next).toHaveLength(SHORT_DESCRIPTION_MAX_LENGTH);
		expect(next?.startsWith(existing)).toBe(true);
	});
});
