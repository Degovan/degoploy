// Incremental parser for deployment logs: splits the byte stream into lines,
// turns ANSI colors into styled spans and groups lines into the runner's steps
// (the "==> " headers degoploy writes before each stage).

import type { StageKey } from '#lib/stages.js';

export type Span = { text: string; cls: string; style?: string };

export interface LogLine {
	/** Byte offset of the line's first byte in the log. */
	offset: number;
	spans: Span[];
	/** Text without escapes, for matching. */
	text: string;
}

export interface LogStep {
	key: StageKey;
	lines: LogLine[];
}

type Sgr = {
	fg: number | string | null;
	bold: boolean;
	dim: boolean;
	italic: boolean;
	underline: boolean;
};

const plain = (): Sgr => ({ fg: null, bold: false, dim: false, italic: false, underline: false });

// Readable on the always-dark console.
const palette = [
	'text-zinc-500',
	'text-red-400',
	'text-emerald-400',
	'text-amber-300',
	'text-sky-400',
	'text-fuchsia-400',
	'text-cyan-400',
	'text-zinc-200',
	'text-zinc-400',
	'text-red-300',
	'text-emerald-300',
	'text-yellow-200',
	'text-sky-300',
	'text-fuchsia-300',
	'text-cyan-300',
	'text-white'
];

function color256(n: number): string {
	if (n < 16) return palette[n];
	if (n >= 232) {
		const v = 8 + (n - 232) * 10;
		return `rgb(${v},${v},${v})`;
	}
	const i = n - 16;
	const c = (x: number) => (x === 0 ? 0 : 55 + x * 40);
	return `rgb(${c(Math.floor(i / 36))},${c(Math.floor(i / 6) % 6)},${c(i % 6)})`;
}

function applySgr(state: Sgr, params: number[]): Sgr {
	const s = { ...state };
	if (params.length === 0) params = [0];
	for (let i = 0; i < params.length; i++) {
		const p = params[i];
		if (p === 0) Object.assign(s, plain());
		else if (p === 1) s.bold = true;
		else if (p === 2) s.dim = true;
		else if (p === 3) s.italic = true;
		else if (p === 4) s.underline = true;
		else if (p === 22) s.bold = s.dim = false;
		else if (p === 23) s.italic = false;
		else if (p === 24) s.underline = false;
		else if (p >= 30 && p <= 37) s.fg = p - 30;
		else if (p >= 90 && p <= 97) s.fg = p - 90 + 8;
		else if (p === 39) s.fg = null;
		else if (p === 38 || p === 48) {
			// Extended colors; backgrounds are parsed only to skip their arguments.
			if (params[i + 1] === 5) {
				if (p === 38) s.fg = color256(params[i + 2] ?? 0);
				i += 2;
			} else if (params[i + 1] === 2) {
				if (p === 38)
					s.fg = `rgb(${params[i + 2] ?? 0},${params[i + 3] ?? 0},${params[i + 4] ?? 0})`;
				i += 4;
			}
		}
	}
	return s;
}

function spanStyle(s: Sgr): { cls: string; style?: string } {
	const cls: string[] = [];
	let style: string | undefined;
	if (typeof s.fg === 'number') cls.push(palette[s.fg]);
	else if (typeof s.fg === 'string') {
		if (s.fg.startsWith('rgb')) style = `color:${s.fg}`;
		else cls.push(s.fg);
	}
	if (s.bold) cls.push('font-semibold');
	if (s.dim) cls.push('opacity-60');
	if (s.italic) cls.push('italic');
	if (s.underline) cls.push('underline');
	return { cls: cls.join(' '), style };
}

// CSI (ESC [ ... final), OSC (ESC ] ... BEL or ESC \) and other two-byte escapes.
// eslint-disable-next-line no-control-regex -- matching terminal escapes is the point
const escapes = /\x1b\[([0-9;?]*)[ -/]*([@-~])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?|\x1b[@-Z\\-_]/g;

/**
 * Renders one line (without its newline) starting in `state`. A carriage return
 * starts the line over, like a terminal redrawing a progress bar; the last
 * non-empty redraw is kept. Returns the spans and the state after the line.
 */
export function renderLine(raw: string, state: Sgr): { spans: Span[]; text: string; state: Sgr } {
	let cur: Span[] = [];
	let kept: Span[] = [];
	let s = state;
	const push = (text: string) => {
		// Control characters other than tab would render as boxes.
		// eslint-disable-next-line no-control-regex -- stripping them is the point
		text = text.replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '');
		if (!text) return;
		const { cls, style } = spanStyle(s);
		const last = cur[cur.length - 1];
		if (last && last.cls === cls && last.style === style) last.text += text;
		else cur.push({ text, cls, style });
	};
	const redraw = () => {
		if (cur.length) kept = cur;
		cur = [];
	};
	let last = 0;
	for (const m of raw.matchAll(escapes)) {
		const before = raw.slice(last, m.index);
		for (const [i, part] of before.split('\r').entries()) {
			if (i > 0) redraw();
			push(part);
		}
		last = m.index + m[0].length;
		if (m[2] === 'm') {
			s = applySgr(
				s,
				m[1]
					.split(';')
					.filter((x) => x !== '')
					.map(Number)
			);
		}
	}
	for (const [i, part] of raw.slice(last).split('\r').entries()) {
		if (i > 0) redraw();
		push(part);
	}
	const spans = cur.length ? cur : kept;
	return { spans, text: spans.map((x) => x.text).join(''), state: s };
}

/** Which step a "==> " header starts, from the texts degoploy's runner writes. */
function headerStep(text: string): StageKey | null {
	if (!text.startsWith('==> ')) return null;
	if (text.startsWith('==> Deployment #')) return 'trigger';
	if (text.startsWith('==> Updating repository')) return 'update';
	if (text.startsWith('==> Running ')) return 'script';
	return null;
}

const decoder = new TextDecoder();

/** Accumulates log bytes; read `steps` and `partial` after each `push`. */
export class LogParser {
	steps: LogStep[] = [];
	/** The unterminated last line (live output), or null. */
	partial: LogLine | null = null;
	#pending = new Uint8Array(0);
	#pendingOffset = 0;
	#state = plain();
	#size = 0;

	/** Bytes received so far. */
	get size() {
		return this.#size;
	}

	push(bytes: Uint8Array) {
		const buf = new Uint8Array(this.#pending.length + bytes.length);
		buf.set(this.#pending);
		buf.set(bytes, this.#pending.length);
		this.#size += bytes.length;

		let start = 0;
		for (let i = 0; i < buf.length; i++) {
			if (buf[i] !== 0x0a) continue;
			this.#addLine(buf.subarray(start, i), this.#pendingOffset + start);
			start = i + 1;
		}
		this.#pendingOffset += start;
		this.#pending = buf.slice(start);
		this.partial = this.#pending.length
			? this.#line(this.#pending, this.#pendingOffset, this.#state).line
			: null;
	}

	#line(bytes: Uint8Array, offset: number, state: Sgr) {
		const r = renderLine(decoder.decode(bytes), state);
		return { line: { offset, spans: r.spans, text: r.text }, state: r.state };
	}

	#addLine(bytes: Uint8Array, offset: number) {
		const { line, state } = this.#line(bytes, offset, this.#state);
		this.#state = state;
		const key = headerStep(line.text);
		if (key) this.steps.push({ key, lines: [] });
		else if (!this.steps.length) this.steps.push({ key: 'trigger', lines: [] });
		this.steps[this.steps.length - 1].lines.push(line);
	}

	/** Lines of a step, with the live partial line if that step is the last one. */
	linesOf(key: StageKey): LogLine[] {
		const step = this.steps.findLast((s) => s.key === key);
		if (!step) return [];
		const isLast = step === this.steps[this.steps.length - 1];
		return isLast && this.partial ? [...step.lines, this.partial] : step.lines;
	}
}

/** Unix ms when the line at `offset` was written, from [offset, ms] entries sorted by offset. */
export function timeAt(entries: number[][], offset: number): number | null {
	let lo = 0;
	let hi = entries.length - 1;
	let found: number | null = null;
	while (lo <= hi) {
		const mid = (lo + hi) >> 1;
		if (entries[mid][0] <= offset) {
			found = entries[mid][1];
			lo = mid + 1;
		} else {
			hi = mid - 1;
		}
	}
	return found;
}

// bash prefixes errors with the script's full path; the summary keeps only its name.
const shortPaths = (s: string) => s.trim().replace(/\S*\/([^/\s:]+):/g, '$1:');

/** Max bytes of the server's one-line error summary (maxLineLen in the runner). */
const ERROR_MAX = 300;

/** Whether a log line is the one the deployment's error summary was taken from. */
export function sameError(line: string, error: string): boolean {
	const l = shortPaths(line);
	const e = shortPaths(error);
	if (!e) return false;
	const cut = new TextEncoder().encode(error).length >= ERROR_MAX - 3;
	return l === e || (cut && l.startsWith(e));
}
