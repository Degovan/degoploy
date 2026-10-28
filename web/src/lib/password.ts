const alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';

/** Random password from an unambiguous alphabet (no 0/O, 1/l/I). */
export function generatePassword(length = 20): string {
	const bytes = crypto.getRandomValues(new Uint8Array(length));
	return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join('');
}

export type Strength = { score: 0 | 1 | 2 | 3 | 4; label: string };

const strengthLabels = ['Too short', 'Weak', 'Fair', 'Good', 'Strong'] as const;

/**
 * Rough strength for the meter, from length and character variety. The server
 * only enforces the 10-character minimum.
 */
export function passwordStrength(pw: string): Strength {
	if (pw.length < 10) return { score: 0, label: strengthLabels[0] };
	const classes = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((r) => r.test(pw)).length;
	const raw = classes - 1 + (pw.length >= 14 ? 1 : 0) + (pw.length >= 18 ? 1 : 0);
	const score = Math.min(4, Math.max(1, raw)) as Strength['score'];
	return { score, label: strengthLabels[score] };
}
