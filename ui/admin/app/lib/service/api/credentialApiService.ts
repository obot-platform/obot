import { z } from "zod";

import { Credential, CredentialNamespace } from "~/lib/model/credentials";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";

const param = (x: string) => x as Todo;

const getCredentialsFetcher = createFetcher(
	z.object({
		namespace: z.nativeEnum(CredentialNamespace),
		entityId: z.string(),
	}),
	async ({ namespace, entityId }, { signal }) => {
		const { url } = ApiRoutes.credentials.getCredentials(namespace, entityId);
		const { data } = await request<EntityList<Credential>>({ url, signal });

		return data.items ?? [];
	},
	() =>
		ApiRoutes.credentials.getCredentials(param(":namespace"), ":entityId").path
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
	getCredentials: getCredentialsFetcher,
	deleteCredential,
};
