import { Credential, CredentialNamespace } from "~/lib/model/credentials";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getCredentials(
    namespace: CredentialNamespace,
    entityId: string
) {
    const { data } = await request<EntityList<Credential>>({
        url: ApiRoutes.credentials.getCredentialsForEntity(namespace, entityId)
            .url,
    });

    return data.items ?? [];
}
getCredentials.key = (
    namespace: CredentialNamespace,
    entityId?: Nullish<string>
) => {
    if (!entityId) return null;

    return {
        url: ApiRoutes.credentials.getCredentialsForEntity(namespace, entityId)
            .path,
        entityId,
        namespace,
    };
};

async function deleteCredential(
    namespace: CredentialNamespace,
    entityId: string,
    credentialId: string
) {
    await request({
        url: ApiRoutes.credentials.deleteCredential(
            namespace,
            entityId,
            credentialId
        ).url,
    });
}

export const CredentialApiService = {
    getCredentials,
    deleteCredential,
};
