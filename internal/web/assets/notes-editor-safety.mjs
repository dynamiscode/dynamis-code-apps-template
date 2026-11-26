export function safeLinkURL(value) {
	if (!/^https?:\/\//iu.test(value) || /[\s\\\u0000-\u001f\u007f]/u.test(value)) return false;
	try {
		const url = new URL(value);
		return (url.protocol === "https:" || url.protocol === "http:") && !!url.hostname && !url.username && !url.password;
	} catch (_) {
		return false;
	}
}

// Restrict visual imports to an explicit dialect; the editor must also round-trip
// the complete original source exactly before it can replace the textarea.
export function safeVisualSource(source) {
	const token = /(?:[^\\`*_~\[\]<>|]+|\*\*[^\\`*_~\[\]<>|\n]+\*\*|\*[^\\`*_~\[\]<>|\n]+\*|\[([^\\`*_~\[\]<>|\n]+)\]\((https?:\/\/[^\s()\\]+)\))/guy;
	for (const line of source.split("\n")) {
		if (!line) continue;
		if (/^\s|^[#>]|^[-=]{3,}\s*$|!\[/u.test(line)) return false;
		const content = line.replace(/^(?:[-*] |\d+\. )/u, "");
		if (/^(?:\+ |\d+\) )/u.test(content)) return false;
		let offset = 0;
		while (offset < content.length) {
			token.lastIndex = offset;
			const match = token.exec(content);
			if (!match || (match[2] && !safeLinkURL(match[2]))) return false;
			offset = token.lastIndex;
		}
	}
	return true;
}
