import { SettingsIcon } from "lucide-react";
import { useState } from "react";

import { OAuthApp } from "~/lib/model/oauthApps";
import {
    OAuthAppSpec,
    OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "../ui/tooltip";
import { ConfigureOAuthApp } from "./ConfigureOAuthApp";
import { DeleteOAuthApp } from "./DeleteOAuthApp";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function OAuthAppDetail({
    type,
    className,
}: {
    type: OAuthProvider;
    className?: string;
}) {
    const spec = useOAuthAppInfo(type);

    const [successModalOpen, setSuccessModalOpen] = useState(false);

    if (!spec) {
        console.error(`OAuth app ${type} not found`);
        return null;
    }

    return (
        <>
            <Dialog>
                <DialogTrigger asChild>
                    <Button
                        size="icon"
                        variant="ghost"
                        className={cn(className)}
                    >
                        <SettingsIcon />
                    </Button>
                </DialogTrigger>

                <DialogDescription hidden>OAuth App Details</DialogDescription>

                <DialogContent>
                    <DialogHeader>
                        <DialogTitle className="flex items-center gap-2">
                            <OAuthAppTypeIcon type={type} />

                            <span>{spec?.displayName}</span>

                            {spec.disableConfiguration && (
                                <span>is not configurable</span>
                            )}
                        </DialogTitle>
                    </DialogHeader>

                    {spec.disableConfiguration ? (
                        <DisabledContent spec={spec} />
                    ) : spec?.appOverride ? (
                        <Content
                            app={spec.appOverride}
                            spec={spec}
                            onSuccess={() => setSuccessModalOpen(true)}
                        />
                    ) : (
                        <EmptyContent
                            spec={spec}
                            onSuccess={() => setSuccessModalOpen(true)}
                        />
                    )}
                </DialogContent>
            </Dialog>

            <Dialog open={successModalOpen} onOpenChange={setSuccessModalOpen}>
                <DialogContent>
                    <DialogTitle>
                        Successfully Configured {spec.displayName} OAuth App
                    </DialogTitle>

                    <DialogDescription>
                        Otto will now use your custom {spec.displayName} OAuth
                        app to authenticate users.
                    </DialogDescription>

                    <DialogFooter>
                        <DialogClose asChild>
                            <Button className="w-full">Close</Button>
                        </DialogClose>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </>
    );
}

function DisabledContent({ spec }: { spec: OAuthAppSpec }) {
    return <TypographyP>{spec.disabledReason}</TypographyP>;
}

function EmptyContent({
    spec,
    onSuccess,
}: {
    spec: OAuthAppSpec;
    onSuccess: () => void;
}) {
    return (
        <div className="flex flex-col gap-2">
            <TypographyP>
                {spec.displayName} OAuth is currently enabled. No action is
                needed here.
            </TypographyP>

            <TypographyP className="mb-4">
                You can also configure your own {spec.displayName} OAuth by
                clicking the button below.
            </TypographyP>

            <ConfigureOAuthApp type={spec.type} onSuccess={onSuccess} />
        </div>
    );
}

function Content({
    app,
    spec,
    onSuccess,
}: {
    app: OAuthApp;
    spec: OAuthAppSpec;
    onSuccess: () => void;
}) {
    return (
        <div className="flex flex-col gap-2">
            <TypographyP>
                Otto only supports one custom {spec.displayName} OAuth. If you
                need to use a different configuration, you can replace the
                current configuration with a new one.
            </TypographyP>

            <TypographyP>
                When {spec.displayName} OAuth is used, Otto will use your custom
                OAuth app.
            </TypographyP>

            <div className="grid grid-cols-2 gap-2 px-8 py-4">
                <TypographyP>
                    <strong>Client ID</strong>
                </TypographyP>
                <TooltipProvider>
                    <Tooltip>
                        <TooltipTrigger className="truncate underline decoration-dotted">
                            {app.clientID}
                        </TooltipTrigger>

                        <TooltipContent>{app.clientID}</TooltipContent>
                    </Tooltip>
                </TooltipProvider>

                <TypographyP>
                    <strong>Client Secret</strong>
                </TypographyP>
                <TypographyP>****************</TypographyP>
            </div>

            <ConfigureOAuthApp type={app.type} onSuccess={onSuccess} />
            <DeleteOAuthApp type={app.type} disableTooltip id={app.id} />
        </div>
    );
}
