import { m } from '$lib/i18n';
import { Role } from './types';

export const userRoleOptions = [
	{
		id: Role.BASIC,
		label: m.core_role_standard_user(),
		description: m.core_role_standard_user_description()
	},
	{
		id: Role.POWERUSER,
		label: m.core_role_power_user(),
		description: m.core_role_power_user_description()
	},
	{
		id: Role.POWERUSER_PLUS,
		label: m.core_role_power_user_plus(),
		description: m.core_role_power_user_plus_description()
	},
	{
		id: Role.ADMIN,
		label: m.core_role_admin(),
		description: m.core_role_admin_description()
	}
];

export const groupRoleOptions = [
	{
		id: Role.ADMIN,
		label: m.core_role_admin(),
		description: m.core_group_role_admin_description()
	},
	{
		id: Role.POWERUSER_PLUS,
		label: m.core_role_power_user_plus(),
		description: m.core_group_role_power_user_plus_description()
	},
	{
		id: Role.POWERUSER,
		label: m.core_role_power_user(),
		description: m.core_group_role_power_user_description()
	}
];

export const OBOT_PLATFORM_REPO = 'https://github.com/obot-platform/';
