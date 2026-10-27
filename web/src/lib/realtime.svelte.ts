// One WebSocket per tab to /ws: topic subscriptions, reconnect with backoff, and
// automatic resubscription (log topics resume from the last byte offset).

export type RealtimeMessage = {
	topic: string;
	type: 'deployment.updated' | 'webhook.received' | 'log.chunk' | 'log.end' | 'error';
	project_id?: number;
	offset?: number;
	data?: unknown;
};

type Handler = (m: RealtimeMessage) => void;
type Subscription = { handler: Handler; offset?: () => number };

export type RealtimeStatus = 'idle' | 'connecting' | 'open' | 'closed';

class Realtime {
	status = $state<RealtimeStatus>('idle');

	#ws: WebSocket | null = null;
	#subs = new Map<string, Set<Subscription>>();
	#wanted = false;
	#retry = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;

	connect() {
		if (this.#wanted) return;
		this.#wanted = true;
		this.#open();
	}

	disconnect() {
		this.#wanted = false;
		clearTimeout(this.#timer);
		this.#ws?.close();
		this.#ws = null;
		this.status = 'idle';
	}

	/** Subscribes handler to topic; returns the unsubscribe function. */
	subscribe(topic: string, handler: Handler, opts: { offset?: () => number } = {}): () => void {
		const sub: Subscription = { handler, offset: opts.offset };
		let set = this.#subs.get(topic);
		const first = !set;
		// Plain Set on purpose: subscriptions are bookkeeping, not UI state.
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		if (!set) this.#subs.set(topic, (set = new Set()));
		set.add(sub);
		if (first) this.#sendSubscribe(topic, sub);
		return () => {
			set.delete(sub);
			if (set.size === 0) {
				this.#subs.delete(topic);
				this.#send({ op: 'unsubscribe', topic });
			}
		};
	}

	/** Re-requests a topic (e.g. after detecting a gap in log offsets). */
	resubscribe(topic: string) {
		const sub = this.#subs.get(topic)?.values().next().value;
		if (sub) this.#sendSubscribe(topic, sub);
	}

	#sendSubscribe(topic: string, sub: Subscription) {
		this.#send({ op: 'subscribe', topic, offset: sub.offset?.() ?? 0 });
	}

	#send(msg: object) {
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(JSON.stringify(msg));
	}

	#open() {
		this.status = 'connecting';
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(`${proto}//${location.host}/ws`);
		this.#ws = ws;

		ws.onopen = () => {
			this.status = 'open';
			this.#retry = 0;
			for (const [topic, set] of this.#subs) {
				const sub = set.values().next().value;
				if (sub) this.#sendSubscribe(topic, sub);
			}
		};
		ws.onmessage = (ev) => {
			let msg: RealtimeMessage;
			try {
				msg = JSON.parse(ev.data);
			} catch {
				return;
			}
			for (const sub of this.#subs.get(msg.topic) ?? []) sub.handler(msg);
		};
		ws.onclose = () => {
			if (this.#ws !== ws) return;
			this.#ws = null;
			this.status = 'closed';
			if (!this.#wanted) return;
			// 1s, 2s, 4s … capped at 30s, with jitter so tabs do not reconnect in lockstep.
			const delay = Math.min(30_000, 1000 * 2 ** this.#retry++) * (0.75 + Math.random() / 2);
			this.#timer = setTimeout(() => this.#open(), delay);
		};
	}
}

export const realtime = new Realtime();

/** Decodes the base64 payload of a log.chunk message. */
export function decodeChunk(data: unknown): Uint8Array {
	const s = typeof data === 'string' ? atob(data) : '';
	const out = new Uint8Array(s.length);
	for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
	return out;
}
