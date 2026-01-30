import { doDelete, doGet, doPost, type Fetcher } from '../http';
import type { APIKey, APIKeyCreateRequest, APIKeyCreateResponse } from './types';

type ItemsResponse<T> = { items: T[] | null };

// User endpoints

export async function listApiKeys(opts?: { fetch?: Fetcher }): Promise<APIKey[]> {
	const response = (await doGet('/api-keys', opts)) as ItemsResponse<APIKey>;
	return response.items ?? [];
}

export async function getApiKey(id: string, opts?: { fetch?: Fetcher }): Promise<APIKey> {
	const response = (await doGet(`/api-keys/${id}`, opts)) as APIKey;
	return response;
}

export async function createApiKey(
	request: APIKeyCreateRequest,
	opts?: { fetch?: Fetcher }
): Promise<APIKeyCreateResponse> {
	const response = (await doPost('/api-keys', request, opts)) as APIKeyCreateResponse;
	return response;
}

export async function deleteApiKey(id: string): Promise<void> {
	await doDelete(`/api-keys/${id}`);
}

// Admin endpoints

export async function listAllApiKeys(opts?: { fetch?: Fetcher }): Promise<APIKey[]> {
	const response = (await doGet('/admin-api-keys', opts)) as ItemsResponse<APIKey>;
	return response.items ?? [];
}

export async function getAnyApiKey(id: string, opts?: { fetch?: Fetcher }): Promise<APIKey> {
	const response = (await doGet(`/admin-api-keys/${id}`, opts)) as APIKey;
	return response;
}

export async function deleteAnyApiKey(id: string): Promise<void> {
	await doDelete(`/admin-api-keys/${id}`);
}
