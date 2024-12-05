import { CircleAlert } from "lucide-react";

import { TypographyP } from "~/components/Typography";

export function ModelProviderBanner() {
    return (
        <div className="flex flex-row p-2 justify-start items-center w-[calc(100%-4rem)] rounded-sm bg-secondary relative overflow-hidden gap-2 max-w-screen-lg">
            <CircleAlert className="text-warning" />
            <div className="flex flex-col gap-1">
                <TypographyP className="font-semibold text-xs">
                    To use Otto&apos;s features, you&apos;ll need to set up a
                    Model Provider. Select and configure one below to get
                    started!
                </TypographyP>
            </div>
        </div>
    );
}
