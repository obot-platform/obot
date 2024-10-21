import { SettingsIcon } from "lucide-react";

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
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import { CreateOauthApp } from "./CreateOauthApp";
import { DeleteOAuthApp } from "./DeleteOAuthApp";
import { EditOAuthApp } from "./EditOAuthApp";
import { OAuthAppTypeIcon } from "./OAuthAppTypeIcon";

export function OAuthAppDetail({
    type,
    className,
}: {
    type: OAuthProvider;
    className?: string;
}) {
    const spec = useOAuthAppInfo(type);

    if (!spec) {
        console.error(`OAuth app ${type} not found`);
        return null;
    }

    return (
        <Dialog>
            <DialogTrigger asChild>
                <Button size="icon" variant="ghost" className={cn(className)}>
                    <SettingsIcon />
                </Button>
            </DialogTrigger>

            <DialogDescription hidden>OAuth App Details</DialogDescription>

            <DialogContent>
                <DialogHeader>
                    <DialogTitle className="flex items-center gap-2">
                        <OAuthAppTypeIcon type={type} />

                        <span>{spec?.displayName}</span>
                    </DialogTitle>
                </DialogHeader>

                {spec?.customApp ? (
                    <Content app={spec.customApp} spec={spec} />
                ) : (
                    <EmptyContent spec={spec} />
                )}
            </DialogContent>
        </Dialog>
    );
}

function EmptyContent({ spec }: { spec: OAuthAppSpec }) {
    return (
        <div className="flex flex-col gap-2">
            <TypographyP>
                {spec.displayName} OAuth is automatically being handled by the
                Acorn Gateway
            </TypographyP>

            <TypographyP className="mb-4">
                If you would like Otto to use your own custom {spec.displayName}{" "}
                OAuth App, you can configure it by clicking the button below.
            </TypographyP>

            <CreateOauthApp type={spec.type} />
        </div>
    );
}

function Content({ app, spec }: { app: OAuthApp; spec: OAuthAppSpec }) {
    return (
        <div className="flex flex-col gap-2">
            <TypographyP>
                You have a custom configuration for {spec.displayName} OAuth.
            </TypographyP>

            <TypographyP>
                When {spec.displayName} OAuth is used, Otto will use your custom
                OAuth app.
            </TypographyP>

            <div className="grid grid-cols-2 gap-2 px-8 py-4">
                <TypographyP>
                    <strong>Client ID</strong>
                </TypographyP>
                <TypographyP>{app.clientID}</TypographyP>

                <TypographyP>
                    <strong>Client Secret</strong>
                </TypographyP>
                <TypographyP>****************</TypographyP>
            </div>

            <EditOAuthApp type={app.type} />
            <DeleteOAuthApp disableTooltip id={app.id} />
        </div>
    );
}
