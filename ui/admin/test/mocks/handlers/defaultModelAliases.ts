import { HttpResponse, http } from "test";
import { mockedDefaultModelAliases } from "test/mocks/models/defaultModelAliases";

import { DefaultModelAlias } from "~/lib/model/defaultModelAliases";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

export const defaultModelAliasHandler = http.get(
	ApiRoutes.defaultModelAliases.getAliases().path,
	() => {
		return HttpResponse.json<EntityList<DefaultModelAlias>>({
			items: mockedDefaultModelAliases,
		});
	}
);
