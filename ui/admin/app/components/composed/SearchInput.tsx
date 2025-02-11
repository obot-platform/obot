import { SearchIcon } from "lucide-react";
import { useState } from "react";

import { Input } from "~/components/ui/input";
import { useDebounce } from "~/hooks/useDebounce";

export function SearchInput({
	onChange,
	placeholder = "Search...",
}: {
	onChange: (value: string) => void;
	placeholder?: string;
}) {
	const [searchQuery, setSearchQuery] = useState("");
	const debounceOnChange = useDebounce(onChange, 300);
	return (
		<div className="relative">
			<Input
				type="text"
				placeholder={placeholder}
				value={searchQuery}
				onChange={(e) => {
					setSearchQuery(e.target.value);
					debounceOnChange(e.target.value);
				}}
				startContent={<SearchIcon className="h-5 w-5 text-gray-400" />}
				className="w-64"
			/>
		</div>
	);
}
