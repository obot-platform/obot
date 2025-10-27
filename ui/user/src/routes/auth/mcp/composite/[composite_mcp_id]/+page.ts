export const load = ({ params, url }: { params: { composite_mcp_id: string }; url: URL }) => {
	console.error(`composite auth page: ${url.toString()}`);
	console.error(`oauth auth request: ${url.searchParams.get('oauth_auth_request')}`);
	return {
		compositeMcpId: params.composite_mcp_id,
		oauthAuthRequestId: url.searchParams.get('oauth_auth_request') || undefined
	};
};
