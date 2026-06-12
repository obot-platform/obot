#!/usr/bin/env node
import crypto from 'node:crypto';
import http from 'node:http';

const port = Number(process.env.PORT || '0');
const issuer = trimRight(process.env.OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER || '', '/');
const clientId = process.env.OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID || '';
const clientSecret = process.env.OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET || '';
const scope = process.env.OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE || 'openid email profile';
const publicURL = trimRight(process.env.OBOT_SERVER_PUBLIC_URL || 'http://127.0.0.1:15174', '/');
const redirectURI = `${publicURL}/oauth2/callback`;

const pending = new Map();
const sessions = new Map();
const tokens = new Map();
let discovery;

const server = http.createServer(async (req, res) => {
	try {
		if (req.method === 'GET' && req.url === '/') {
			res.writeHead(200, { 'content-type': 'text/plain' });
			res.end('ok');
			return;
		}

		const url = new URL(req.url || '/', `http://${req.headers.host}`);
		if (req.method === 'GET' && url.pathname === '/oauth2/start') {
			await handleStart(url, res);
			return;
		}
		if (req.method === 'GET' && url.pathname === '/oauth2/callback') {
			await handleCallback(url, res);
			return;
		}
		if (req.method === 'GET' && url.pathname === '/oauth2/sign_out') {
			res.writeHead(302, {
				'set-cookie': 'obot_access_token=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax',
				location: safeRedirect(url.searchParams.get('rd'))
			});
			res.end();
			return;
		}
		if (req.method === 'POST' && url.pathname === '/obot-get-state') {
			await handleGetState(req, res);
			return;
		}
		if (req.method === 'GET' && url.pathname === '/obot-get-user-info') {
			handleGetUserInfo(req, res);
			return;
		}

		res.writeHead(404, { 'content-type': 'text/plain' });
		res.end('not found');
	} catch (err) {
		res.writeHead(500, { 'content-type': 'text/plain' });
		res.end(err?.stack || String(err));
	}
});

server.listen(port, '127.0.0.1');

async function handleStart(url, res) {
	const doc = await getDiscovery();
	const state = crypto.randomUUID();
	const nonce = crypto.randomUUID();
	pending.set(state, {
		nonce,
		rd: safeRedirect(url.searchParams.get('rd'))
	});

	const authorizationURL = new URL(doc.authorization_endpoint);
	authorizationURL.searchParams.set('response_type', 'code');
	authorizationURL.searchParams.set('client_id', clientId);
	authorizationURL.searchParams.set('redirect_uri', redirectURI);
	authorizationURL.searchParams.set('scope', scope);
	authorizationURL.searchParams.set('state', state);
	authorizationURL.searchParams.set('nonce', nonce);

	res.writeHead(302, { location: authorizationURL.toString() });
	res.end();
}

async function handleCallback(url, res) {
	const state = url.searchParams.get('state') || '';
	const code = url.searchParams.get('code') || '';
	const request = pending.get(state);
	pending.delete(state);
	if (!request || !code) {
		res.writeHead(400, { 'content-type': 'text/plain' });
		res.end('invalid oauth callback');
		return;
	}

	const doc = await getDiscovery();
	const tokenResponse = await postForm(doc.token_endpoint, {
		grant_type: 'authorization_code',
		code,
		redirect_uri: redirectURI,
		client_id: clientId,
		client_secret: clientSecret
	});

	const profile = await resolveProfile(doc, tokenResponse);
	const sessionID = crypto.randomUUID();
	const accessToken = tokenResponse.access_token || `fixture-access-token-${sessionID}`;
	const emailVerified =
		typeof profile.email_verified === 'boolean' ? profile.email_verified : undefined;
	sessions.set(sessionID, {
		accessToken,
		preferredUsername: profile.preferred_username || profile.name || profile.email || profile.sub,
		user: `iss:${issuer}\u0000sub:${profile.sub}`,
		email: profile.email || '',
		issuer,
		emailVerified
	});
	tokens.set(accessToken, profile);

	res.writeHead(302, {
		'set-cookie': `obot_access_token=${sessionID}; Path=/; HttpOnly; SameSite=Lax`,
		location: request.rd
	});
	res.end();
}

async function handleGetState(req, res) {
	const body = JSON.parse(await readBody(req));
	const cookie = parseCookie(body.header?.Cookie?.[0] || body.header?.cookie?.[0] || '');
	const session = sessions.get(cookie.obot_access_token);
	if (!session) {
		res.writeHead(500, { 'content-type': 'text/plain' });
		res.end('record not found');
		return;
	}

	res.writeHead(200, { 'content-type': 'application/json' });
	res.end(JSON.stringify(session));
}

function handleGetUserInfo(req, res) {
	const token = (req.headers.authorization || '').replace(/^Bearer\s+/i, '');
	const profile = tokens.get(token);
	if (!profile) {
		res.writeHead(401, { 'content-type': 'text/plain' });
		res.end('unknown token');
		return;
	}

	res.writeHead(200, { 'content-type': 'application/json' });
	res.end(JSON.stringify(profile));
}

async function getDiscovery() {
	if (!discovery) {
		const response = await fetch(`${issuer}/.well-known/openid-configuration`);
		if (!response.ok) {
			throw new Error(`discovery failed: ${response.status} ${response.statusText}`);
		}
		discovery = await response.json();
	}
	return discovery;
}

async function resolveProfile(doc, tokenResponse) {
	const idTokenClaims = decodeJWT(tokenResponse.id_token);
	if (doc.userinfo_endpoint && tokenResponse.access_token) {
		const response = await fetch(doc.userinfo_endpoint, {
			headers: { authorization: `Bearer ${tokenResponse.access_token}` }
		});
		if (response.ok) {
			return { ...idTokenClaims, ...(await response.json()) };
		}
	}
	return idTokenClaims;
}

async function postForm(url, form) {
	const response = await fetch(url, {
		method: 'POST',
		headers: { 'content-type': 'application/x-www-form-urlencoded' },
		body: new URLSearchParams(form)
	});
	if (!response.ok) {
		throw new Error(`token exchange failed: ${response.status} ${response.statusText}`);
	}
	return response.json();
}

function decodeJWT(token) {
	if (!token) {
		return {};
	}
	const [, payload] = token.split('.');
	return JSON.parse(Buffer.from(payload, 'base64url').toString('utf8'));
}

function parseCookie(header) {
	return Object.fromEntries(
		header
			.split(';')
			.map((part) => part.trim())
			.filter(Boolean)
			.map((part) => {
				const index = part.indexOf('=');
				return [part.slice(0, index), decodeURIComponent(part.slice(index + 1))];
			})
	);
}

function readBody(req) {
	return new Promise((resolve, reject) => {
		let body = '';
		req.setEncoding('utf8');
		req.on('data', (chunk) => {
			body += chunk;
		});
		req.on('end', () => resolve(body));
		req.on('error', reject);
	});
}

function safeRedirect(rd) {
	return rd && rd.startsWith('/') ? rd : '/';
}

function trimRight(value, suffix) {
	while (value.endsWith(suffix)) {
		value = value.slice(0, -suffix.length);
	}
	return value;
}
