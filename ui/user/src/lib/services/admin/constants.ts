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
		id: Role.ADMIN_READONLY,
		label: m.core_role_admin_readonly(),
		description: m.core_role_admin_readonly_description()
	},
	{
		id: Role.ADMIN,
		label: m.core_role_admin(),
		description: m.core_role_admin_description()
	}
];

// Roles that can be the default role for new users. Admin Read-Only can only be assigned explicitly.
export const defaultRoleOptions = userRoleOptions.filter((role) => role.id !== Role.ADMIN_READONLY);

export const groupRoleOptions = [
	{
		id: Role.ADMIN,
		label: m.core_role_admin(),
		description: m.core_group_role_admin_description()
	},
	{
		id: Role.ADMIN_READONLY,
		label: m.core_role_admin_readonly(),
		description: m.core_group_role_admin_readonly_description()
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
