import { Credential, CredentialNamespace } from "~/lib/model/credentials";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes, createRevalidate } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getCredentials(
	namespace: CredentialNamespace,
	entityId: string
) {
	const { data } = await request<EntityList<Credential>>({
		url: ApiRoutes.credentials.getCredentials(namespace, entityId).url,
	});

	return data.items ?? [];
}
getCredentials.key = (
	namespace: CredentialNamespace,
	entityId?: Nullish<string>
) => {
	if (!entityId) return null;

	return {
		url: ApiRoutes.credentials.getCredentials(namespace, entityId).path,
		entityId,
		namespace,
	};
};
getCredentials.revalidate = createRevalidate(
	ApiRoutes.credentials.getCredentials
);

async function deleteCredential(
	namespace: CredentialNamespace,
	entityId: string,
	credentialName: string
) {
	await request({
		url: ApiRoutes.credentials.deleteCredential(
			namespace,
			entityId,
			credentialName
		).url,
		method: "DELETE",
	});

	return credentialName;
}

export const CredentialApiService = {
	getCredentials,
	deleteCredential,
};
