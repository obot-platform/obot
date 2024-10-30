export type User = {
    id: number;
    created: Date;
    username: string;
    email: string;
    role: Role;
    iconURL: string;
};

export const Role = {
    Admin: 1,
    Default: 2,
} as const;
export type Role = (typeof Role)[keyof typeof Role];

export function roleToString(role: Role): string {
    return (
        Object.keys(Role).find(
            (key) => Role[key as keyof typeof Role] === role
        ) || "Unknown"
    );
}

export function stringToRole(roleStr: string): Role {
    const role = Role[roleStr as keyof typeof Role];
    if (role === undefined) {
        throw new Error(`Invalid role string: ${roleStr}`);
    }
    return role;
}
