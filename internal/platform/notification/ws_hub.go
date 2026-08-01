package notification

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

const (
	// Час на запис повідомлення в сокет
	writeWait = 10 * time.Second

	// Час очікування наступного Pong від клієнта
	pongWait = 60 * time.Second

	// Період відправки Ping повідомлень (має бути менше за pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Максимальний розмір повідомлення (не очікуємо великих від клієнта)
	maxMessageSize = 1024
)

// Hub керує набором активних клієнтів та маршрутизує повідомлення.
type Hub struct {
	mu          sync.RWMutex
	connections map[uuid.UUID][]*Client
	logger      logger.Logger
}

// NewHub створює новий екземпляр Hub.
func NewHub(l logger.Logger) *Hub {
	return &Hub{
		connections: make(map[uuid.UUID][]*Client),
		logger:      l,
	}
}

// RegisterClient додає клієнта до хабу.
func (h *Hub) RegisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[client.userID] = append(h.connections[client.userID], client)
	h.logger.Debugw("WebSocket client registered", "user_id", client.userID)
}

// UnregisterClient безпечно видаляє клієнта з хабу.
func (h *Hub) UnregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.connections[client.userID]
	if !ok {
		return
	}

	for i, c := range clients {
		if c == client {
			// Видаляємо клієнта з масиву
			h.connections[client.userID] = append(clients[:i], clients[i+1:]...)
			// Закриваємо канал відправки (це зупинить writePump)
			close(client.send)
			h.logger.Debugw("WebSocket client unregistered", "user_id", client.userID)
			break
		}
	}

	if len(h.connections[client.userID]) == 0 {
		delete(h.connections, client.userID)
	}
}

// NotifyUser відправляє повідомлення всім клієнтам вказаного користувача.
// Цей метод безпечний для одночасного виклику з різних горутин.
func (h *Hub) NotifyUser(userID uuid.UUID, data interface{}) error {
	h.mu.RLock()
	clients, ok := h.connections[userID]
	h.mu.RUnlock()

	if !ok {
		return nil
	}

	h.logger.Debugw("WebSocket notification: sending to clients", "user_id", userID, "count", len(clients))
	for _, client := range clients {
		select {
		case client.send <- data:
		default:
			// Якщо черга клієнта переповнена — це ознака проблемного з'єднання
			h.logger.Warnw("Client send buffer full, dropping connection", "user_id", userID)
			go h.UnregisterClient(client)
		}
	}
	return nil
}

// Client представляє окреме WebSocket-з'єднання.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan interface{}
	userID uuid.UUID
}

// NewClient ініціалізує новий об'єкт клієнта.
func NewClient(hub *Hub, conn *websocket.Conn, userID uuid.UUID) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan interface{}, 256),
		userID: userID,
	}
}

// ReadPump вичитує повідомлення з сокета.
// В основному використовується для підтримки з'єднання (Pong) та виявлення розриву.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.UnregisterClient(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Errorw("WebSocket read error", "error", err, "user_id", c.userID)
			}
			break
		}
	}
}

// WritePump слухає канал send та пересилає повідомлення у сокет.
// Також відповідає за відправку Ping повідомлень для перевірки зв'язку.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub закрив канал
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				c.hub.logger.Errorw("WebSocket write error", "error", err, "user_id", c.userID)
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
