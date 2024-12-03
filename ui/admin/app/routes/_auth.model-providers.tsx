import { preload } from "swr";

import { ModelProviderService } from "~/lib/service/api/modelProviderApiService";

import { TypographyH2 } from "~/components/Typography";
import { ModelProviderList } from "~/components/model-providers/ModelProviderLists";

export async function clientLoader() {
    await preload(
        ModelProviderService.getModelProviders.key(),
        ModelProviderService.getModelProviders
    );

    return null;
}

export default function ModelProviders() {
    return (
        <div className="relative space-y-10 px-8 pb-8">
            <div className="sticky top-0 bg-background pt-8 pb-4 flex items-center justify-between">
                <TypographyH2 className="mb-4">Model Providers</TypographyH2>
            </div>

            <div className="h-full flex flex-col gap-8 overflow-hidden">
                <ModelProviderList />
            </div>
        </div>
    );
}
