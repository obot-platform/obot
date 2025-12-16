import { Role } from './types';

export const userRoleOptions = [
	{
		id: Role.BASIC,
		label: 'Basic User',
		description: 'Connect to MCP servers made available through registries and use Chat.'
	},
	{
		id: Role.POWERUSER,
		label: 'Power User',
		description:
			'In addition to basic user features, users can publish custom MCP servers for their own personal use.'
	},
	{
		id: Role.POWERUSER_PLUS,
		label: 'Power User Plus',
		description:
			'In addition to power user features, users can share their custom MCP servers through their own registries.'
	},
	{
		id: Role.ADMIN,
		label: 'Admin',
		description: 'Every user is a full admin. Use caution when selecting this option.'
	}
];
