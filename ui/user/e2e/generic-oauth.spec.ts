import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { spawn, type ChildProcess } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { OAuth2Server } from 'oauth2-mock-server';

type GenericOAuthFixture = {
	provider: {
		id: string;
		namespace: string;
		name: string;
		clientId: string;
		clientSecret: string;
		scope: string;
		emailDomains: string;
		trustEmailLinking: boolean;
	};
	user: {
		subject: string;
		email: string;
		emailVerified: boolean;
		preferredUsername: string;
		name: string;
		picture: string;
	};
	bootstrap: {
		token: string;
	};
};

type KeycloakFixture = {
	enabledUnless: {
		env: string;
		value: string;
	};
	container: {
		image: string;
		realm: string;
		port: number;
	};
	provider: GenericOAuthFixture['provider'];
	user: {
		username: string;
		password: string;
		email: string;
		emailVerified: boolean;
		firstName: string;
		lastName: string;
	};
};

const fixturePath = new URL('./fixtures/generic-oauth.mock.json', import.meta.url);
const keycloakSettingsPath = new URL('./fixtures/keycloak.settings.json', import.meta.url);
const keycloakRealmPath = new URL('./fixtures/keycloak.realm.json', import.meta.url);
const recordVideo = process.env.OBOT_E2E_RECORD_VIDEO === 'true';
const recordPauseMs = Number(process.env.OBOT_E2E_RECORD_PAUSE_MS ?? '1200');
let fixture: GenericOAuthFixture;
let oidcServer: OAuth2Server;

test.beforeAll(async ({ request }) => {
	fixture = JSON.parse(await readFile(fixturePath, 'utf8')) as GenericOAuthFixture;
	oidcServer = await startMockOIDC(fixture);
	if (!recordVideo) {
		await configureGenericProvider(request, fixture, oidcServer.issuer.url ?? '');
	}
});

test.afterAll(async () => {
	await oidcServer?.stop();
});

test('mock OIDC provider can be configured and used for login', async ({ page }) => {
	if (recordVideo) {
		test.setTimeout(180_000);
		await configureGenericProviderInUI(page, fixture, oidcServer.issuer.url ?? '');
	}

	await page.goto('/');
	await recordPause(page);

	await expect(
		page.getByRole('button', { name: `Continue with ${fixture.provider.name}` })
	).toBeVisible();
	await recordPause(page);
	await page.getByRole('button', { name: `Continue with ${fixture.provider.name}` }).click();
	await recordPause(page);

	await expect(page).toHaveURL(/\/dashboard/);
	await recordPause(page);

	const me = await page.request.get('/api/me');
	expect(me.ok()).toBeTruthy();
	const profile = await me.json();
	expect(profile.email).toBe(fixture.user.email);
	expect(profile.username).toContain(`sub:${fixture.user.subject}`);
});

test('Keycloak credential flow can log in through generic OAuth', async ({ page, request }) => {
	test.skip(
		process.env.OBOT_E2E_KEYCLOAK === 'false',
		'OBOT_E2E_KEYCLOAK=false disables the heavier local Keycloak credential-entry flow in CI'
	);

	const keycloakFixture = JSON.parse(
		await readFile(keycloakSettingsPath, 'utf8')
	) as KeycloakFixture;
	const keycloak = await startKeycloak(keycloakFixture);
	const keycloakIssuer = `http://127.0.0.1:${keycloakFixture.container.port}/realms/${keycloakFixture.container.realm}`;
	const keycloakGenericFixture: GenericOAuthFixture = {
		provider: keycloakFixture.provider,
		user: {
			subject: keycloakFixture.user.username,
			email: keycloakFixture.user.email,
			emailVerified: keycloakFixture.user.emailVerified,
			preferredUsername: keycloakFixture.user.username,
			name: `${keycloakFixture.user.firstName} ${keycloakFixture.user.lastName}`,
			picture: ''
		},
		bootstrap: fixture.bootstrap
	};

	try {
		if (recordVideo) {
			test.setTimeout(180_000);
			await configureGenericProviderInUI(page, keycloakGenericFixture, keycloakIssuer);
		} else {
			await configureGenericProvider(request, keycloakGenericFixture, keycloakIssuer);
		}

		await page.goto('/');
		await page
			.getByRole('button', { name: `Continue with ${keycloakFixture.provider.name}` })
			.click();
		await page.getByLabel(/username|email/i).fill(keycloakFixture.user.username);
		await page.getByRole('textbox', { name: 'Password' }).fill(keycloakFixture.user.password);
		await page.getByRole('button', { name: /sign in/i }).click();

		await expect(page).toHaveURL(/\/dashboard/);
		const me = await page.request.get('/api/me');
		expect(me.ok()).toBeTruthy();
		const profile = await me.json();
		expect(profile.email).toBe(keycloakFixture.user.email);
	} finally {
		keycloak.kill('SIGINT');
	}
});

async function startMockOIDC(fixture: GenericOAuthFixture) {
	const server = new OAuth2Server();
	await server.issuer.keys.generate('RS256');
	server.service.on('beforeTokenSigning', (token) => {
		token.payload.sub = fixture.user.subject;
		token.payload.email = fixture.user.email;
		token.payload.email_verified = fixture.user.emailVerified;
		token.payload.preferred_username = fixture.user.preferredUsername;
		token.payload.name = fixture.user.name;
		token.payload.picture = fixture.user.picture;
		token.payload.aud = fixture.provider.clientId;
	});
	server.service.on('beforeUserinfo', (response) => {
		response.body = {
			sub: fixture.user.subject,
			email: fixture.user.email,
			email_verified: fixture.user.emailVerified,
			preferred_username: fixture.user.preferredUsername,
			name: fixture.user.name,
			picture: fixture.user.picture
		};
	});
	await server.start(0, '127.0.0.1');
	return server;
}

async function configureGenericProviderInUI(
	page: Page,
	fixture: GenericOAuthFixture,
	issuer: string
) {
	await page.goto('/admin');
	await recordPause(page);
	await page.locator('input[name="bootstrap-token"]').fill(fixture.bootstrap.token);
	await recordPause(page);
	await page.getByRole('button', { name: 'Login' }).click();

	await expect(page).toHaveURL(/\/admin\/dashboard/);
	await recordPause(page);
	await page.goto('/admin/auth-providers');
	await expect(page.locator('#initial-loader.loaded')).toBeVisible({ timeout: 30_000 });
	await closeOpenDialogs(page);
	await expect(page.getByRole('heading', { name: 'Custom OAuth / OIDC' })).toBeVisible();
	await recordPause(page);
	await closeOpenDialogs(page);

	await page
		.getByRole('heading', { name: 'Custom OAuth / OIDC' })
		.locator('xpath=ancestor::div[1]')
		.getByRole('button', { name: 'Configure' })
		.click();

	await expect(page.getByText('Set Up Custom OAuth / OIDC')).toBeVisible();
	await recordPause(page);
	await page.locator('#OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME').fill(fixture.provider.name);
	await recordPause(page);
	await page.locator('#OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER').fill(issuer);
	await recordPause(page);
	await page.locator('#OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID').fill(fixture.provider.clientId);
	await recordPause(page);
	await page
		.locator('input[name="OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET"]')
		.fill(fixture.provider.clientSecret);
	await recordPause(page);
	await page.locator('#OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE').fill(fixture.provider.scope);
	await recordPause(page);
	await page.locator('#OBOT_AUTH_PROVIDER_EMAIL_DOMAINS input').fill(fixture.provider.emailDomains);
	await page.locator('#OBOT_AUTH_PROVIDER_EMAIL_DOMAINS input').press('Enter');
	await recordPause(page);
	await page.getByRole('button', { name: 'Confirm' }).click();
	await recordPause(page);

	await page.request.post('/api/bootstrap/logout');
	await page.goto('/oauth2/sign_out?rd=/');
	await page.waitForURL('/');
	await recordPause(page);
}

async function closeOpenDialogs(page: Page) {
	if ((await page.locator('dialog[open]').count()) === 0) {
		return;
	}
	await page.locator('dialog[open]').evaluateAll((dialogs) => {
		for (const dialog of dialogs) {
			(dialog as HTMLDialogElement).close();
		}
	});
	await expect(page.locator('dialog[open]')).toHaveCount(0);
}

async function recordPause(page: Page) {
	if (!recordVideo || recordPauseMs <= 0) {
		return;
	}
	await page.waitForTimeout(recordPauseMs);
}

async function configureGenericProvider(
	request: APIRequestContext,
	fixture: GenericOAuthFixture,
	issuer: string
) {
	const headers = {
		Authorization: `Bearer ${fixture.bootstrap.token}`
	};
	const providerID = fixture.provider.id;
	await request.post(`/api/auth-providers/${providerID}/deconfigure`, { headers });
	await expect
		.poll(async () => {
			const response = await request.get('/api/auth-providers', { headers });
			if (!response.ok()) {
				return false;
			}
			const body = await response.json();
			return Boolean(body.items?.some((provider: { id: string }) => provider.id === providerID));
		})
		.toBe(true);

	const configureResponse = await request.post(`/api/auth-providers/${providerID}/configure`, {
		headers,
		data: {
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME: fixture.provider.name,
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER: issuer,
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID: fixture.provider.clientId,
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET: fixture.provider.clientSecret,
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE: fixture.provider.scope,
			OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: fixture.provider.emailDomains,
			OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING: String(
				fixture.provider.trustEmailLinking
			),
			PATH: process.env.PATH ?? ''
		}
	});
	expect(configureResponse.ok()).toBeTruthy();

	await expect
		.poll(async () => {
			const response = await request.get('/api/auth-providers');
			if (!response.ok()) {
				return '';
			}
			const body = await response.json();
			return body.items?.find((provider: { id: string }) => provider.id === providerID)?.name ?? '';
		})
		.toBe(fixture.provider.name);
}

async function startKeycloak(fixture: KeycloakFixture): Promise<ChildProcess> {
	const realmPath = fileURLToPath(keycloakRealmPath);
	const child = spawn(
		'docker',
		[
			'run',
			'--rm',
			'--name',
			`obot-e2e-keycloak-${process.pid}-${Date.now()}`,
			'-p',
			`${fixture.container.port}:8080`,
			'-e',
			'KC_BOOTSTRAP_ADMIN_USERNAME=admin',
			'-e',
			'KC_BOOTSTRAP_ADMIN_PASSWORD=admin',
			'-e',
			'KEYCLOAK_ADMIN=admin',
			'-e',
			'KEYCLOAK_ADMIN_PASSWORD=admin',
			'-v',
			`${realmPath}:/opt/keycloak/data/import/realm.json:ro`,
			fixture.container.image,
			'start-dev',
			'--import-realm',
			'--http-enabled=true',
			'--hostname-strict=false'
		],
		{ stdio: 'inherit' }
	);

	await waitForURL(
		`http://127.0.0.1:${fixture.container.port}/realms/${fixture.container.realm}/.well-known/openid-configuration`,
		120_000
	);
	return child;
}

async function waitForURL(url: string, timeout: number) {
	const started = Date.now();
	let lastError: unknown;
	while (Date.now() - started < timeout) {
		try {
			const response = await fetch(url);
			if (response.ok) {
				return;
			}
			lastError = new Error(`HTTP ${response.status}`);
		} catch (err) {
			lastError = err;
		}
		await new Promise((resolve) => setTimeout(resolve, 1_000));
	}
	throw new Error(`timed out waiting for ${url}: ${lastError}`);
}
