import { BoxesIcon, CircleCheckIcon, CircleSlashIcon } from "lucide-react";
import useSWR from "swr";

import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

import { ModelProviderConfigure } from "~/components/model-providers/ModelProviderConfigure";
import { Card, CardContent, CardHeader } from "~/components/ui/card";

export function ModelProviderList() {
    const { data: modelProviders } = useSWR(
        ModelProviderApiService.getModelProviders.key(),
        () => ModelProviderApiService.getModelProviders(),
        { fallbackData: [] }
    );

    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4">
                {modelProviders.map((modelProvider) => (
                    <Card key={modelProvider.id}>
                        <CardHeader className="pb-0 flex flex-row justify-end">
                            <ModelProviderConfigure
                                modelProvider={modelProvider}
                            />
                        </CardHeader>
                        <CardContent className="flex flex-col items-center gap-4">
                            <div className="w-16 h-16">
                                <BoxesIcon className="w-16 h-16 color-primary" />
                            </div>
                            <div className="text-lg font-semibold">
                                {modelProvider.name}
                            </div>
                            <div className="w-full flex flex-row items-center justify-center gap-1 text-sm bg-secondary rounded-sm p-2">
                                {modelProvider.configured ? (
                                    <CircleCheckIcon className="w-4 h-4 text-success" />
                                ) : (
                                    <CircleSlashIcon className="w-4 h-4 text-destructive" />
                                )}{" "}
                                {modelProvider.configured
                                    ? "Configured"
                                    : "Not Configured"}
                            </div>
                        </CardContent>
                    </Card>
                ))}
            </div>
        </div>
    );
}
