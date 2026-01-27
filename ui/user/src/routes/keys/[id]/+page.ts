import type { PageLoad } from './$types';
import { ApiKeysService } from '$lib/services';
import { handleRouteError } from '$lib/errors';

export const load: PageLoad = async ({ params, parent, fetch }) => {
    const { profile } = await parent();
    const { id } = params;
    let apiKey;
    try {
        apiKey = await ApiKeysService.getApiKey(id, { fetch });
    } catch (err) {
        handleRouteError(err, `/admin/api-keys/${id}`, profile);
    }
    return {
        apiKey,
    };
};
