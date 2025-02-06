import { useMemo, useState } from "react";
import { MetaFunction } from "react-router";
import useSWR, { preload } from "swr";

import { convertToolReferencesToMap } from "~/lib/model/toolReferences";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { SearchInput } from "~/components/composed/SearchInput";
import { CreateTool } from "~/components/tools/CreateTool";
import { filterToolCatalogBySearch } from "~/components/tools/ToolCatalog";
import { ToolGrid } from "~/components/tools/toolGrid";
import { ScrollArea } from "~/components/ui/scroll-area";

export async function clientLoader() {
	await Promise.all([
		preload(ToolReferenceService.getToolReferences.key("tool"), () =>
			ToolReferenceService.getToolReferences("tool")
		),
		preload(OauthAppService.getOauthApps.key(), () =>
			OauthAppService.getOauthApps()
		),
	]);
	return null;
}

export default function Tools() {
	const getTools = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		() => ToolReferenceService.getToolReferences("tool"),
		{ fallbackData: [] }
	);

	const toolMap = useMemo(
		() => convertToolReferencesToMap(getTools.data, true),
		[getTools.data]
	);

	const [searchQuery, setSearchQuery] = useState("");

	const results =
		searchQuery.length > 0
			? filterToolCatalogBySearch(Object.entries(toolMap), searchQuery)
			: Object.entries(toolMap);

	return (
		<div>
			<div className="flex items-center justify-between px-8 pt-8">
				<h2>Tools</h2>
				<div className="flex items-center space-x-2">
					<SearchInput
						onChange={(value) => setSearchQuery(value)}
						placeholder="Search for tools..."
					/>
					<CreateTool />
				</div>
			</div>

			<ScrollArea className="flex h-[calc(100vh-8.5rem)] flex-col p-8">
				<ToolGrid toolMap={results} />
			</ScrollArea>
		</div>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Tools" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Tools` }];
};
