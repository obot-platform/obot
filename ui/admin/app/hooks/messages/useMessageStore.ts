import { useEffect, useState } from "react";
import { useStore } from "zustand";

import { createMessageStore } from "~/lib/store/chat/message-store";

export const useInitMessageStore = (threadId: Nullish<string>) => {
	const [storeObj] = useState(() => createMessageStore());
	const store = useStore(storeObj);

	const { init, reset } = store;
	useEffect(() => {
		if (!threadId) return;

		init(threadId);

		return () => reset();
	}, [init, reset, threadId]);

	return store;
};
