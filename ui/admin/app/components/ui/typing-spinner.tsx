import { useEffect, useState } from "react";

import { cn } from "~/lib/utils";

export const TypingDots = ({ className }: { className?: string }) => {
    const [show, setShow] = useState([true]);

    useEffect(() => {
        if (show.length === 3) return;

        const interval = setInterval(
            () => setShow((prevState) => [...prevState, true]),
            200
        );

        return () => clearInterval(interval);
    }, [show]);

    return (
        <div className={cn("flex gap-2 items-center", className)}>
            <style>
                {`
                    .typing-dot {
                        width: 6px;
                        height: 6px;
                        border-radius: 50%;
                        background-color: hsl(var(--foreground));
                        animation: typing-dot 1s infinite;
                    }

                    @keyframes typing-dot {
                        0% {
                            opacity: .2;
                        }
                        50% {
                            opacity: .8;
                        }
                        100% {
                            opacity: .2;
                        }
                    }
                `}
            </style>

            {show[0] && <div className="typing-dot" />}
            {show[1] && <div className="typing-dot" />}
            {show[2] && <div className="typing-dot" />}
        </div>
    );
};
