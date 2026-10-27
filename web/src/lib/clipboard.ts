/**
 * Copies text. navigator.clipboard only exists in secure contexts (https/localhost),
 * so plain-http access by IP falls back to a hidden textarea + execCommand.
 */
export async function copyText(text: string): Promise<boolean> {
	if (navigator.clipboard && window.isSecureContext) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			// fall through
		}
	}
	const ta = document.createElement('textarea');
	ta.value = text;
	ta.setAttribute('readonly', '');
	ta.style.position = 'fixed';
	ta.style.opacity = '0';
	document.body.appendChild(ta);
	ta.select();
	try {
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		ta.remove();
	}
}
