export const load = ({ params, url }: { params: { composite_mcp_id: string }; url: URL }) => {
	return {
		compositeMcpId: params.composite_mcp_id,
		vmcpId: url.searchParams.get('vmcp_id') ?? undefined,
		oauthAuthRequestId: url.searchParams.get('oauth_auth_request') ?? undefined
	};
};
