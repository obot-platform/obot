import { Link } from "@remix-run/react";
import { CircleCheckIcon, CircleSlashIcon } from "lucide-react";

import { ModelProvider } from "~/lib/model/modelProviders";

import { ModelProviderConfigure } from "~/components/model-providers/ModelProviderConfigure";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { ModelProvidersModels } from "~/components/model-providers/ModelProviderModels";
import { ModelProviderLinks } from "~/components/model-providers/constants";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent, CardHeader } from "~/components/ui/card";

export function ModelProviderList({
    modelProviders,
}: {
    modelProviders: ModelProvider[];
}) {
    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4">
                {modelProviders.map((modelProvider) => (
                    <Card key={modelProvider.id}>
                        <CardHeader className="pb-0 flex flex-row justify-end">
                            {modelProvider.configured ? (
                                <ModelProvidersModels
                                    modelProvider={modelProvider}
                                />
                            ) : (
                                <div className="w-9 h-9" />
                            )}
                        </CardHeader>
                        <CardContent className="flex flex-col items-center gap-4">
                            <Link to={ModelProviderLinks[modelProvider.id]}>
                                <ModelProviderIcon
                                    modelProvider={modelProvider}
                                    size="lg"
                                />
                            </Link>
                            <div className="text-lg font-semibold">
                                {modelProvider.name}
                            </div>
                            <Badge
                                className="pointer-events-none"
                                variant="outline"
                            >
                                {modelProvider.configured ? (
                                    <span className="flex gap-1 items-center">
                                        <CircleCheckIcon
                                            size={18}
                                            className="text-success"
                                        />{" "}
                                        Configured
                                    </span>
                                ) : (
                                    <span className="flex gap-1 items-center">
                                        <CircleSlashIcon
                                            size={18}
                                            className="text-destructive"
                                        />
                                        Not Configured
                                    </span>
                                )}
                            </Badge>
                            <ModelProviderConfigure
                                modelProvider={modelProvider}
                            />
                        </CardContent>
                    </Card>
                ))}
            </div>
        </div>
    );
}
