import { CommonAuthProviderId, CommonAuthProviderIds } from "~/lib/model/auth";
import { EntityMeta } from "~/lib/model/primitives";

export type User = EntityMeta & {
	username: string;
	email: string;
	role: Role;
	iconURL: string;
	timezone: string;
	explicitAdmin: boolean;
	currentAuthProvider?: CommonAuthProviderId;
	lastActiveDay?: string; // date
};

export const Role = {
	Admin: 1,
	User: 10,
} as const;
export type Role = (typeof Role)[keyof typeof Role];

const RoleLabels = { [Role.Admin]: "Admin", [Role.User]: "User" };

export const roleLabel = (role: Role) => RoleLabels[role] || "Unknown";
export const roleFromString = (role: string) => {
	const r = +role as Role;

	if (isNaN(r) || !Object.values(Role).includes(r))
		throw new Error("Invalid role");

	return r;
};

export const ExplicitAdminDescription =
	"This user is explicitly set as an admin at the system level and their role cannot be changed.";

export function getUserDisplayName(user?: User) {
	if (user?.currentAuthProvider === CommonAuthProviderIds.GITHUB) {
		return user.username;
	}

	return user?.email;
}
