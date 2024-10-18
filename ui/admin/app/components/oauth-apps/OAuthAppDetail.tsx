import { QuestionMarkCircledIcon } from "@radix-ui/react-icons";
import { CheckCircle2Icon, ClipboardIcon, SettingsIcon } from "lucide-react";
import { ReactNode, useEffect, useState } from "react";
import { toast } from "sonner";

import { OAuthApp } from "~/lib/model/oauthApps";
import {
    OAuthProvider,
    OAuthSingleAppSpec,
} from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { TypographyP, TypographySmall } from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useOAuthAppInfo } from "~/hooks/oauthApps";

import { CreateOauthApp } from "./CreateOauthApp";
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
                    <DialogTitle className="flex items-start gap-2">
                        <OAuthAppTypeIcon type={type} />

                        <span>{spec?.displayName}</span>
                    </DialogTitle>
                </DialogHeader>

                {spec?.customApp ? (
                    <Content oauthApp={spec.customApp} spec={spec} />
                ) : (
                    <EmptyContent spec={spec} />
                )}
            </DialogContent>
        </Dialog>
    );
}

function EmptyContent({ spec }: { spec: OAuthSingleAppSpec }) {
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

function Content({
    oauthApp,
}: {
    spec: OAuthSingleAppSpec;
    oauthApp: OAuthApp;
}) {
    const [copied, setCopied] = useState<string | null>(null);

    useEffect(() => {
        const timeout = setTimeout(() => setCopied(null), 6000);
        return () => clearTimeout(timeout);
    }, [copied]);

    return (
        <div className="flex flex-col gap-2">
            <div className="flex justify-between mt-6">
                <Item
                    label="Reference Name"
                    info="Advanced: Reference names can make your OAuth App's URLs easier to read by allowing you to select the text displayed in the url."
                >
                    <TypographyP
                        className={cn("px-2 rounded-md w-fit", {
                            "bg-success text-success-foreground":
                                oauthApp.refNameAssigned && oauthApp.refName,
                            "bg-error text-error-foreground":
                                !oauthApp.refNameAssigned && oauthApp.refName,
                            "bg-none text-foreground": !oauthApp.refName,
                        })}
                    >
                        {oauthApp.refName ?? "None"}
                    </TypographyP>
                </Item>

                {oauthApp.refName && (
                    <Item label="Assigned">
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger className="underline underline-offset-2 decoration-dotted text-end">
                                    {oauthApp.refNameAssigned ? "Yes" : "No"}
                                </TooltipTrigger>

                                <TooltipContent>
                                    {oauthApp.refNameAssigned
                                        ? "The reference name is currently active"
                                        : "The reference name is not assigned because another OAuth App is using it"}
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </Item>
                )}
            </div>

            {Object.entries(oauthApp.links).map(([key, value]) => {
                // "camelCase" to "Display Name"
                const displayName =
                    key
                        .split("URL")[0]
                        .replace(/([A-Z])/g, " $1")
                        .replace(/^./, (str) => str.toUpperCase()) + " URL";

                return (
                    <Item key={key} label={displayName} className="mt-4 gap-0">
                        <div className="flex items-center gap-2 w-full">
                            <TooltipProvider>
                                <Tooltip>
                                    <TooltipTrigger
                                        onClick={() => copyToClipboard(value)}
                                        className="flex-auto decoration-dotted underline-offset-4 underline text-ellipsis overflow-hidden text-nowrap"
                                    >
                                        {value}
                                    </TooltipTrigger>

                                    <TooltipContent>{value}</TooltipContent>
                                </Tooltip>
                            </TooltipProvider>

                            <Button
                                size="icon"
                                onClick={() => copyToClipboard(value)}
                                className="aspect-square"
                            >
                                {copied === value ? (
                                    <CheckCircle2Icon className="text-success" />
                                ) : (
                                    <ClipboardIcon />
                                )}
                            </Button>
                        </div>
                    </Item>
                );
            })}

            <DeleteOAuthApp id={oauthApp.id} disableTooltip>
                <Button variant="destructive" className="w-full mt-4">
                    Delete OAuth App
                </Button>
            </DeleteOAuthApp>
        </div>
    );

    async function copyToClipboard(value: string) {
        try {
            await navigator.clipboard.writeText(value);

            toast.success("Copied to clipboard");
            setCopied(value);
        } catch (_) {
            toast.error("Failed to copy");
        }
    }
}

function Item({
    label,
    children,
    className,
    info,
}: {
    label: string;
    children: ReactNode;
    className?: string;
    info?: string;
}) {
    return (
        <div className={cn("flex flex-col gap-1", className)}>
            <div className="flex gap-2">
                <TypographySmall>
                    <b>{label}</b>
                </TypographySmall>

                {info && (
                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger>
                                <QuestionMarkCircledIcon />
                            </TooltipTrigger>

                            <TooltipContent className="max-w-[300px]">
                                {info}
                            </TooltipContent>
                        </Tooltip>
                    </TooltipProvider>
                )}
            </div>

            {children}
        </div>
    );
}
