import { CommonAuthProviderId, CommonAuthProviderIds } from "~/lib/model/auth";
import { EntityMeta } from "~/lib/model/primitives";

export type User = EntityMeta & {
	username: string;
	email: string;
	role: Role;
	iconURL: string;
	timezone: string;
	explicitRole: boolean;
	currentAuthProvider?: CommonAuthProviderId;
	lastActiveDay?: string; // date
};

export const Role = {
	Basic: 4,
	Owner: 8,
	Admin: 16,
} as const;
export type Role = (typeof Role)[keyof typeof Role];

const RoleLabels = {
	[Role.Owner]: "Owner",
	[Role.Admin]: "Admin",
	[Role.Basic]: "User",
};

export const roleLabel = (role: Role) => RoleLabels[role] || "Unknown";
export const roleFromString = (role: string) => {
	const r = +role as Role;

	if (isNaN(r) || !Object.values(Role).includes(r))
		throw new Error("Invalid role");

	return r;
};

export const ExplicitRoleDescription =
	"This user's role is explicitly set at the system level and cannot be changed.";

export function getUserDisplayName(user?: User) {
	if (user?.currentAuthProvider === CommonAuthProviderIds.GITHUB) {
		return user.username;
	}

	return user?.email;
}
