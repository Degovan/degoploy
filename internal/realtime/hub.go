// Package realtime fans server events out to WebSocket clients by topic.
package realtime

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
)

// TopicEvents carries deployment and webhook updates for every project the
// client may view. Log output goes to DeploymentLogTopic(id).
const TopicEvents = "events"

func DeploymentLogTopic(id int64) string { return "deployment:" + strconv.FormatInt(id, 10) + ":log" }

// Message is the server → client envelope.
type Message struct {
	Topic     string `json:"topic"`
	Type      string `json:"type"`
	ProjectID int64  `json:"project_id,omitempty"`
	Offset    int64  `json:"offset,omitempty"`
	// Time is when a live log chunk was written (Unix ms); backfilled chunks have none.
	Time int64 `json:"time,omitempty"`
	Data any   `json:"data,omitempty"`
}

const (
	TypeDeploymentUpdated = "deployment.updated"
	TypeWebhookReceived   = "webhook.received"
	TypeLogChunk          = "log.chunk" // Data: base64 bytes starting at Offset
	TypeLogEnd            = "log.end"   // Offset: final size
	TypeError             = "error"
)

// Client is one WebSocket connection. CanView decides per project which events
// it receives; it is evaluated at publish time so role changes apply live.
type Client struct {
	send    chan []byte
	CanView func(projectID int64) bool

	mu     sync.Mutex
	topics map[string]bool
	closed bool
}

// sendBuffer is how many messages may wait for a slow client before it is dropped.
const sendBuffer = 512

func NewClient(canView func(int64) bool) *Client {
	return &Client{send: make(chan []byte, sendBuffer), CanView: canView, topics: map[string]bool{}}
}

// Outbox delivers encoded messages; it is closed when the hub drops the client.
func (c *Client) Outbox() <-chan []byte { return c.send }

func (c *Client) subscribed(topic string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.topics[topic]
}

// trySend queues b, reporting false if the client is gone or its buffer is full.
func (c *Client) trySend(b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}

func (c *Client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewHub() *Hub { return &Hub{clients: map[*Client]struct{}{}} }

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.close()
}

// Subscribe adds topic for c. Authorisation is the caller's job.
func (h *Hub) Subscribe(c *Client, topic string) {
	c.mu.Lock()
	c.topics[topic] = true
	c.mu.Unlock()
}

func (h *Hub) Unsubscribe(c *Client, topic string) {
	c.mu.Lock()
	delete(c.topics, topic)
	c.mu.Unlock()
}

// Send delivers m to a single client (e.g. log backfill or errors).
func (h *Hub) Send(c *Client, m Message) bool {
	b, err := json.Marshal(m)
	if err != nil {
		slog.Error("encode realtime message", "err", err)
		return false
	}
	if !c.trySend(b) {
		h.Unregister(c)
		return false
	}
	return true
}

// Publish delivers m to every client subscribed to m.Topic that may view m.ProjectID.
// Clients that cannot keep up are disconnected; they resync on reconnect.
func (h *Hub) Publish(m Message) {
	b, err := json.Marshal(m)
	if err != nil {
		slog.Error("encode realtime message", "err", err)
		return
	}
	h.mu.RLock()
	var slow []*Client
	for c := range h.clients {
		if !c.subscribed(m.Topic) || (m.ProjectID != 0 && !c.CanView(m.ProjectID)) {
			continue
		}
		if !c.trySend(b) {
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		slog.Warn("dropping slow realtime client")
		h.Unregister(c)
	}
}

// CloseAll disconnects every client (used on shutdown).
func (h *Hub) CloseAll() {
	h.mu.Lock()
	clients := h.clients
	h.clients = map[*Client]struct{}{}
	h.mu.Unlock()
	for c := range clients {
		c.close()
	}
}
