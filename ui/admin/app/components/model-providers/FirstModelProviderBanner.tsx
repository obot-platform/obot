import { useLocation } from "@remix-run/react";
import { useState } from "react";

import { TypographyP } from "~/components/Typography";
import { OttoLogo } from "~/components/branding/OttoLogo";
import { FirstModelProviderConfigure } from "~/components/model-providers/FirstModelProviderConfigure";
import { FirstModelProviderSuccess } from "~/components/model-providers/FirstModelProviderSuccess";
import { useModelProviders } from "~/components/model-providers/ModelProviderContext";

const keywordToPathnameMap = {
    "/agents": "agent",
    "/workflows": "workflow",
    "/models": "model",
    "/webhooks": "webhook",
};

type KeyPathnameType = keyof typeof keywordToPathnameMap;

export function FirstModelProviderBanner() {
    const { configured } = useModelProviders();
    const location = useLocation();

    const [showSuccessDialog, setShowSuccessDialog] = useState(false);

    const validDisplayName =
        keywordToPathnameMap[location.pathname as KeyPathnameType];

    return (
        <div className="w-full">
            {configured || !validDisplayName ? null : (
                <div className="flex justify-center w-full">
                    <div className="flex flex-row p-4 min-h-36 justify-end items-center w-[calc(100%-4rem)] rounded-sm mx-8 mt-4 bg-secondary relative overflow-hidden gap-4 max-w-screen-md">
                        <OttoLogo
                            hideText
                            classNames={{
                                root: "absolute opacity-45 top-[-5rem] left-[-7.5rem]",
                                image: "h-80 w-80",
                            }}
                        />
                        <div className="flex flex-col pl-48">
                            <TypographyP className="font-semibold text-2xl mb-0.5">
                                {`Ready to create your first ${validDisplayName}?`}
                            </TypographyP>
                            <TypographyP className="text-sm font-light mb-2">
                                You&apos;re almost there! To start creating or
                                using {validDisplayName}s, you&apos;ll need
                                access to a LLM (Large Language Model){" "}
                                <b>Model Provider</b>. Luckily, we support a
                                variety of providers to help get you started.
                            </TypographyP>
                            <FirstModelProviderConfigure
                                onSuccess={() => {
                                    console.log("success");
                                    setShowSuccessDialog(true);
                                }}
                            />
                        </div>
                    </div>
                </div>
            )}
            <FirstModelProviderSuccess
                open={showSuccessDialog}
                onClose={(open: boolean) => {
                    if (!open) {
                        setShowSuccessDialog(false);
                    }
                }}
            />
        </div>
    );
}
