import { Link } from "@remix-run/react";
import { CircleCheckIcon, CircleSlashIcon } from "lucide-react";

import { ModelProvider } from "~/lib/model/modelProviders";

import { ModelProviderConfigure } from "~/components/model-providers/ModelProviderConfigure";
import { ModelProviderIcon } from "~/components/model-providers/ModelProviderIcon";
import { ModelProviderLinks } from "~/components/model-providers/constants";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent } from "~/components/ui/card";

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
                        <CardContent className="flex flex-col items-center gap-4 pt-6">
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
