import { FC, useEffect, useRef, useState } from "react";

import {
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceType,
    getKnowledgeSourceType,
    getToolRefForKnowledgeSource,
} from "~/lib/model/knowledge";
import { AgentService } from "~/lib/service/api/agentService";

import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";

interface OauthSignDialogProps {
    agentId: string;
    sourceType: KnowledgeSourceType;
    knowledgeSource: KnowledgeSource;
}

const OauthSignDialog: FC<OauthSignDialogProps> = ({
    agentId,
    sourceType,
    knowledgeSource,
}) => {
    const [isAuthDialogOpen, setIsAuthDialogOpen] = useState(false);
    const [isAuthCheckingDialogOpen, setIsAuthCheckingDialogOpen] =
        useState(false);
    const [oauthUrl, setOauthUrl] = useState("");
    const [authChecked, setAuthChecked] = useState(false);
    // Use a ref to track if only run the auth check once
    const isAuthInitiated = useRef(false);

    useEffect(() => {
        setIsAuthCheckingDialogOpen(
            !authChecked &&
                knowledgeSource.state === KnowledgeFileState.Pending &&
                (getKnowledgeSourceType(knowledgeSource) ===
                    KnowledgeSourceType.Notion ||
                    getKnowledgeSourceType(knowledgeSource) ===
                        KnowledgeSourceType.OneDrive)
        );
    }, [authChecked, knowledgeSource]);

    useEffect(() => {
        if (isAuthInitiated.current) {
            return;
        }

        const fetchOauthUrl = async () => {
            const toolRef = getToolRefForKnowledgeSource(sourceType);
            const authStatus = await AgentService.getAuthUrlForAgent(
                agentId,
                toolRef
            );
            if (authStatus?.required && authStatus?.url) {
                setOauthUrl(authStatus.url);
            } else {
                setOauthUrl("");
            }
            setAuthChecked(true);
        };

        try {
            isAuthInitiated.current = true;
            fetchOauthUrl();
        } catch (error) {
            console.error("Error fetching oauth url", error);
        }
    }, [agentId, sourceType]);

    useEffect(() => {
        if (oauthUrl === "") {
            setIsAuthDialogOpen(false);
        } else {
            setIsAuthCheckingDialogOpen(false);
            setIsAuthDialogOpen(true);
        }
    }, [oauthUrl]);

    return (
        <>
            {authChecked ? (
                <Dialog
                    open={isAuthDialogOpen}
                    onOpenChange={setIsAuthDialogOpen}
                >
                    <DialogContent>
                        <DialogTitle>Please Sign In to Continue</DialogTitle>
                        <p>
                            To access the {sourceType} knowledge source, please
                            sign in.
                        </p>
                        <Button
                            className="w-full"
                            variant="secondary"
                            onClick={() => {
                                window.open(oauthUrl, "_blank");
                                setIsAuthDialogOpen(false);
                            }}
                        >
                            <KnowledgeSourceAvatar
                                knowledgeSourceType={sourceType}
                            />
                            Sign In
                        </Button>
                    </DialogContent>
                </Dialog>
            ) : (
                <Dialog
                    open={isAuthCheckingDialogOpen}
                    onOpenChange={setIsAuthCheckingDialogOpen}
                >
                    <DialogContent>
                        <DialogTitle>Checking Authentication</DialogTitle>
                        <div className="flex flex-row items-center justify-center">
                            <p>
                                Please wait while it is checking authentication.
                            </p>
                            <LoadingSpinner className="ml-2" />
                        </div>
                    </DialogContent>
                </Dialog>
            )}
        </>
    );
};

export default OauthSignDialog;
