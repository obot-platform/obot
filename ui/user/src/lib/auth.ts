export type AuthProvider = {
	configured: boolean;
	icon?: string;
	name: string;
	namespace: string;
	id: string;
};

export async function listAuthProviders(): Promise<AuthProvider[]> {
	const resp = await fetch('/api/auth-providers');
	const data = await resp.json();
	return data.items.filter((provider: AuthProvider) => provider.configured);
}
