import { Link, useLocation } from "react-router";
import { $path } from "safe-routes";

import { assetUrl } from "~/lib/utils";

import { TypographyH3, TypographyP } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

export function FirstModelProviderBanner() {
    const { configured: modelProviderConfigured } = useModelProviders();
    const location = useLocation();
    const isModelsProviderPage = location.pathname.includes(
        $path("/model-providers")
    );

    return isModelsProviderPage || modelProviderConfigured ? null : (
        <div className="w-full">
            <div className="flex justify-center mx-8 mt-4 py-4 bg-secondary overflow-hidden rounded-xl">
                <div className="flex flex-row min-h-36 items-center justify-between w-[calc(100%-4rem)] rounded-sm relative gap-4 max-w-screen-md">
                    <div className="absolute opacity-5 md:opacity-45 md:top-[-1.75rem] md:left-[-3.0rem]">
                        <img
                            alt="Obot Alert"
                            className="md:h-[17.5rem] md:w-[17.5rem]"
                            src={assetUrl(
                                "logo/obot-icon-surprised-yellow.svg"
                            )}
                        />
                    </div>
                    <div className="flex flex-col md:ml-64 relative z-10 gap-1">
                        <TypographyH3 className="mb-0.5">
                            Wait! You need to set up a Model Provider!
                        </TypographyH3>
                        <TypographyP className="text-sm font-light mb-2">
                            You&apos;re almost there! To start creating or using{" "}
                            Obot&apos;s features, you&apos;ll need access to an
                            LLM (Large Language Model) <b>Model Provider</b>.
                            Luckily, we support a variety of providers to help
                            get you started.
                        </TypographyP>
                        <Button className="mt-0 w-fit px-10" variant="warning">
                            <Link to={$path("/model-providers")}>
                                Get Started
                            </Link>
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    );
}
