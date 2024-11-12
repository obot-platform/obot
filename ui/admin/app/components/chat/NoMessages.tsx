import { BrainCircuit, Compass, Wrench } from "lucide-react";

import { useChat } from "~/components/chat/ChatContext";
import { Button } from "~/components/ui/button";

export function NoMessages() {
    const { processUserMessage, isInvoking } = useChat();

    return (
        <div className="flex flex-col items-center justify-center space-y-4 text-center p-4 h-full">
            <h2 className="text-2xl font-semibold">Start the conversation!</h2>
            <p className="text-gray-500">
                Looking for a starting point? Try one of these options.
            </p>
            <div className="flex flex-wrap justify-center gap-2">
                <Button
                    variant="outline"
                    shape="pill"
                    disabled={isInvoking}
                    onClick={() =>
                        processUserMessage(
                            "Tell me who you are and what your objectives are."
                        )
                    }
                >
                    <Compass className="w-4 h-4 mr-2" />
                    Objectives
                </Button>
                <Button
                    variant="outline"
                    shape="pill"
                    disabled={isInvoking}
                    onClick={() =>
                        processUserMessage(
                            "Tell me what tools you have available."
                        )
                    }
                >
                    <Wrench className="w-4 h-4 mr-2" />
                    Tools
                </Button>
                <Button
                    variant="outline"
                    shape="pill"
                    disabled={isInvoking}
                    onClick={() =>
                        processUserMessage(
                            "Using your knowledge tools, tell me about your knowledge set."
                        )
                    }
                >
                    <BrainCircuit className="w-4 h-4 mr-2" />
                    Knowledge
                </Button>
            </div>
        </div>
    );
}
