import { FC, useEffect, useRef, useState } from "react";

import {
    KnowledgeSourceType,
    getToolRefForKnowledgeSource,
} from "~/lib/model/knowledge";
import { AgentService } from "~/lib/service/api/agentService";

import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";

interface OauthSignDialogProps {
    agentId: string;
    sourceType: KnowledgeSourceType;
}

const OauthSignDialog: FC<OauthSignDialogProps> = ({ agentId, sourceType }) => {
    const [isOpen, setIsOpen] = useState(false);
    const [oauthUrl, setOauthUrl] = useState("");
    const isAuthInitiated = useRef(false);

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
        };

        try {
            isAuthInitiated.current = true;
            fetchOauthUrl();
        } catch (error) {
            console.error("Error fetching oauth url", error);
        }
    }, [agentId, sourceType]);

    useEffect(() => {
        if (oauthUrl) {
            setIsOpen(true);
        } else {
            setIsOpen(false);
        }
    }, [oauthUrl]);

    return (
        <Dialog open={isOpen} onOpenChange={setIsOpen}>
            <DialogContent>
                <DialogTitle>Please Sign In to Continue</DialogTitle>
                <p>
                    To access the {sourceType} knowledge source, please sign in.
                </p>
                <Button
                    className="w-full"
                    variant="secondary"
                    onClick={() => {
                        window.open(oauthUrl, "_blank");
                        setIsOpen(false);
                    }}
                >
                    <KnowledgeSourceAvatar knowledgeSourceType={sourceType} />
                    Sign In
                </Button>
            </DialogContent>
        </Dialog>
    );
};

export default OauthSignDialog;
