import { Link } from "@remix-run/react";
import { $path } from "remix-routes";

import { TypographyH3, TypographyP } from "~/components/Typography";
import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";

export function FirstModelProviderBanner() {
    return (
        <div className="w-full">
            <div className="flex justify-center mx-8 mt-4 py-4 bg-secondary overflow-hidden rounded-xl">
                <div className="flex flex-row min-h-36 items-center justify-between w-[calc(100%-4rem)] rounded-sm relative gap-4 max-w-screen-md">
                    <ObotLogo
                        hideText
                        classNames={{
                            root: "absolute opacity-45 top-[-5rem] left-[-7.5rem]",
                            image: "h-80 w-80",
                        }}
                    />
                    <div className="flex flex-col ml-64">
                        <TypographyH3 className="mb-0.5">
                            Ready to create your first Agent?
                        </TypographyH3>
                        <TypographyP className="text-sm font-light mb-2">
                            You&apos;re almost there! To start creating or using{" "}
                            agents, you&apos;ll need access to an LLM (Large
                            Language Model) <b>Model Provider</b>. Luckily, we
                            support a variety of providers to help get you
                            started.
                        </TypographyP>
                        <Button className="mt-0 w-fit px-10">
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
