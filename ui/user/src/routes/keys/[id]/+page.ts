import { redirect } from '@sveltejs/kit';

export const load = async ({ params }) => {
	const { id } = params;
	throw redirect(301, `/agent-auth-scopes/${id}`);
};
