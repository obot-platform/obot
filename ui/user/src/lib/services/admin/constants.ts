import { Role } from './types';

export const userRoleOptions = [
	{
		id: Role.BASIC,
		label: 'Basic User',
		description:
			'New users can connect to MCP servers through the My Connectors app and have access to Obot Chat.'
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
			'In addition to power user features, users can share their custom MCP servers through their own Access Control Rules.'
	},
	{
		id: Role.ADMIN,
		label: 'Admin',
		description: 'Every user is a full admin. Use caution when selecting this option.'
	}
];
