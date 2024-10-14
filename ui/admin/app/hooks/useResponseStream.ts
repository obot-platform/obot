import { useEffect, useRef, useState } from "react";

export function useResponseStream<TData>({
    reader,
    onChunk,
    onComplete,
}: {
    reader: Nullish<ReadableStreamDefaultReader<string>>;
    onChunk: (data: TData) => void;
    onComplete?: () => void;
}) {
    const isRunning = useRef(false);
    const [isComplete, setIsComplete] = useState(false);

    useEffect(() => {
        if (!reader || isRunning.current) return;

        readStream(reader, onChunk, () => setIsComplete(true));
    }, [reader, onChunk]);

    useEffect(() => {
        if (!isComplete) return;

        isRunning.current = false;
        onComplete?.();
        setIsComplete(false);
    }, [isComplete, onComplete]);
}

async function readStream<T>(
    reader: ReadableStreamDefaultReader<string>,
    callback: (data: T) => void,
    onComplete?: () => void
) {
    // eslint-disable-next-line no-constant-condition
    while (true) {
        const { value, done } = await reader.read();

        if (done) break;

        const data = JSON.parse(value.split("data: ")[1]);

        callback(data);

        // todo(tylerslaton): this is a hack to make the stream not miss chunks. since there
        // is no latency in the stream the chunks are being dropped. need to find a better
        // solution to this. Chunks are still being dropped with this, just at a lower rate.
        await new Promise((resolve) => setTimeout(resolve, 200));
    }

    onComplete?.();
}
