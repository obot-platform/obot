import { useChat } from "~/components/chat/ChatContext";
import { Button } from "~/components/ui/button";

export function NoMessages() {
    const { processUserMessage } = useChat();

    const handleAddMessage = (content: string) => {
        processUserMessage(content, "user");
    };

    return (
        <div className="flex flex-col items-center justify-center space-y-4 text-center p-4">
            <h2 className="text-2xl font-semibold">No messages yet</h2>
            <p className="text-gray-500">
                Start the conversation with a sample message:
            </p>
            <div className="flex flex-wrap justify-center gap-2">
                <Button
                    variant="secondary"
                    onClick={() => handleAddMessage("Hello, how are you?")}
                >
                    👋 Greeting
                </Button>
                <Button
                    variant="secondary"
                    onClick={() => handleAddMessage("What can you do?")}
                >
                    🚀 Capabilities
                </Button>
                <Button
                    variant="secondary"
                    onClick={() => handleAddMessage("Tell me a joke")}
                >
                    😄 Joke
                </Button>
            </div>
        </div>
    );
}
