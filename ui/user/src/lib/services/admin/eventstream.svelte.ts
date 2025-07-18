export class EventStreamService<T> {
	private eventSource: EventSource | null = null;

	connect(
		url: string,
		options?: {
			onMessage?: (data: T) => void;
			onOpen?: () => void;
			onError?: (error: Event) => void;
			onClose?: () => void;
		}
	) {
		this.eventSource = new EventSource(url);

		this.eventSource.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				options?.onMessage?.(data);
			} catch (_error) {
				options?.onMessage?.(event.data as T);
			}
		};

		// Handle custom event types (like 'log', 'ping', etc.)
		this.eventSource.addEventListener('log', (event) => {
			try {
				const data = JSON.parse(event.data);
				options?.onMessage?.(data);
			} catch (_error) {
				options?.onMessage?.(event.data as T);
			}
		});

		this.eventSource.onopen = () => {
			console.log('SSE connection opened');
			options?.onOpen?.();
		};

		this.eventSource.onerror = (error) => {
			console.error('SSE connection error:', error);
			options?.onError?.(error);
		};

		// Custom close event handling
		this.eventSource.addEventListener('close', () => {
			console.log('SSE connection closed by server');
			options?.onClose?.();
		});
	}

	disconnect() {
		this.eventSource?.close();
		this.eventSource = null;
	}
}
