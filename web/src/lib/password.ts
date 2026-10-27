const alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';

/** Random password from an unambiguous alphabet (no 0/O, 1/l/I). */
export function generatePassword(length = 20): string {
	const bytes = crypto.getRandomValues(new Uint8Array(length));
	return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join('');
}
