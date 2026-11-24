import { createHttpError } from '$lib/errors';
import { AgentIdentitiesService } from '$lib/services';
import { load } from './+page';
import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => vi.restoreAllMocks());

const signedIn = { loaded: true, isBootstrapUser: () => false };

function run(url: string, profile: object = signedIn) {
	return load({
		fetch: vi.fn(),
		url: new URL(url),
		params: { requestID: 'adr1req' },
		parent: vi.fn(async () => ({ profile }))
	} as unknown as Parameters<typeof load>[0]);
}

describe('agent approval page load', () => {
	it('sends a signed-out user to sign in, passing the login hint, and back to the page', async () => {
		const get = vi.spyOn(AgentIdentitiesService, 'getAgentDelegationRequest');

		await expect(
			run('https://obot.example/agents/approve/adr1req?login_hint=ada%40example.com', {
				unauthorized: true
			})
		).rejects.toMatchObject({
			status: 303,
			location:
				'/?rd=%2Fagents%2Fapprove%2Fadr1req%3Flogin_hint%3Dada%2540example.com&login_hint=ada%40example.com'
		});
		expect(get).not.toHaveBeenCalled();
	});

	it('sends a user whose session ended to sign in', async () => {
		vi.spyOn(AgentIdentitiesService, 'getAgentDelegationRequest').mockRejectedValue(
			createHttpError(401, '/agent-delegation-requests/adr1req', 'unauthorized')
		);

		await expect(run('https://obot.example/agents/approve/adr1req')).rejects.toMatchObject({
			status: 303,
			location: '/?rd=%2Fagents%2Fapprove%2Fadr1req'
		});
	});

	it('returns the request a signed-in user may approve', async () => {
		const request = { id: 'adr1req' };
		vi.spyOn(AgentIdentitiesService, 'getAgentDelegationRequest').mockResolvedValue(
			request as never
		);

		await expect(
			run('https://obot.example/agents/approve/adr1req?oauth_error=access_denied')
		).resolves.toMatchObject({
			request,
			returnPath: '/agents/approve/adr1req?oauth_error=access_denied',
			oauthError: 'access_denied'
		});
	});

	it.each([403, 404])('returns the server message of a %d without redirecting', async (status) => {
		vi.spyOn(AgentIdentitiesService, 'getAgentDelegationRequest').mockRejectedValue(
			createHttpError(
				status,
				'/agent-delegation-requests/adr1req',
				"This agent isn't allowed to act on your behalf."
			)
		);

		const result = await run('https://obot.example/agents/approve/adr1req');
		expect(result).toMatchObject({ error: "This agent isn't allowed to act on your behalf." });
		expect(result).not.toHaveProperty('request');
	});

	it('refuses the bootstrap login without asking the server', async () => {
		const get = vi.spyOn(AgentIdentitiesService, 'getAgentDelegationRequest');

		const result = await run('https://obot.example/agents/approve/adr1req', {
			loaded: true,
			isBootstrapUser: () => true
		});
		expect(result).toMatchObject({
			error: 'Sign in with your own account to approve an agent.'
		});
		expect(get).not.toHaveBeenCalled();
	});
});
