import { EntityMeta } from "~/lib/model/primitives";

export type User = EntityMeta & {
	username: string;
	email: string;
	role: Role;
	iconURL: string;
	timezone: string;
	explicitAdmin: boolean;
};

export const Role = {
	Admin: 1,
	Default: 10,
} as const;
export type Role = (typeof Role)[keyof typeof Role];

const RoleLabels = { [Role.Admin]: "Admin", [Role.Default]: "Default" };

export const roleLabel = (role: Role) => RoleLabels[role] || "Unknown";
export const roleFromString = (role: string) => {
	const r = +role as Role;

	if (isNaN(r) || !Object.values(Role).includes(r))
		throw new Error("Invalid role");

	return r;
};
