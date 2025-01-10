import { useCallback, useRef, useState } from "react";

export function useRefState<T>(initialValue: T) {
	const ref = useRef(initialValue);
	const [state, _setState] = useState(initialValue);

	const setState = useCallback((value: T) => {
		ref.current = value;
		_setState(value);
	}, []);

	return [ref, state, setState] as const;
}
