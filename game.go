package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

const maxPlayers = 8

type player struct {
	id   string
	name string
	conn *websocket.Conn
	send chan Message
}

func newPlayer(id string, name string, conn *websocket.Conn) *player {
	return &player{
		id:   id,
		name: name,
		conn: conn,
		send: make(chan Message, 32),
	}
}

type Message struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

type gameMessage struct {
	Message
	p *player
}

type game struct {
	slug          string
	players       map[string]*player
	mu            sync.Mutex
	broadcast     chan gameMessage
	currentPlayer *player
	currentWord   string
}

func newGame(slug string) *game {
	return &game{
		slug:      slug,
		players:   make(map[string]*player),
		broadcast: make(chan gameMessage, 64),
	}
}

func (g *game) addPlayer(p *player) {
	g.mu.Lock()
	g.players[p.id] = p
	g.mu.Unlock()

	fmt.Println("new player", p.id, "in game", g.slug)

	go p.readLoop(g)
	go p.writeLoop()

	g.broadcastMessage(gameMessage{
		Message: Message{
			Type: "player_joined",
			Payload: map[string]any{
				"id":   p.id,
				"name": p.name,
			},
		},
	})
}

func (g *game) removePlayer(p *player) {
	g.mu.Lock()
	if _, ok := g.players[p.id]; !ok {
		g.mu.Unlock()
		return
	}
	delete(g.players, p.id)
	g.mu.Unlock()

	close(p.send)
	p.conn.Close()

	g.broadcastMessage(gameMessage{
		Message: Message{
			Type: "player_left",
			Payload: map[string]any{
				"id": p.id,
			},
		},
	})
}

func (p *player) readLoop(g *game) {
	defer g.removePlayer(p)

	for {
		_, msg, err := p.conn.ReadMessage()
		if err != nil {
			return
		}

		var jsonMsg Message
		if err := json.Unmarshal(msg, &jsonMsg); err != nil {
			fmt.Println("Invalid JSON:", err)
			continue
		}

		g.broadcast <- gameMessage{
			Message: jsonMsg,
			p:       p,
		}
	}
}

func (p *player) writeLoop() {
	for msg := range p.send {
		if err := p.conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

func (g *game) numPlayers() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.players)
}

func (g *game) begin() {

	g.mu.Lock()
	for _, v := range g.players {
		g.currentPlayer = v
		break
	}
	g.mu.Unlock()

	g.currentWord = "sea"

	fmt.Println("game started with word:", g.currentWord)

	for {
		msg := <-g.broadcast

		switch msg.Type {

		case "draw":

			g.mu.Lock()
			isDrawer := g.currentPlayer != nil && msg.p != nil && msg.p.id == g.currentPlayer.id
			g.mu.Unlock()

			if isDrawer {
				g.broadcastMessage(msg)
			}

		case "chat":
			raw := msg.Payload["message"]
			guess, ok := raw.(string)

			if !ok {
				fmt.Println("chat not string")
				continue
			}

			if guess == g.currentWord {
				g.broadcastMessage(gameMessage{
					p: msg.p,
					Message: Message{
						Type: "word_match",
						Payload: map[string]any{
							"id": msg.p.id,
						},
					},
				})
			} else {
				g.broadcastMessage(msg)
			}

		default:
			g.broadcastMessage(msg)
		}
	}
}

func (g *game) broadcastMessage(msg gameMessage) {

	newPayload := make(map[string]any, len(msg.Payload))
	for k, v := range msg.Payload {
		newPayload[k] = v
	}

	final := Message{
		Type:    msg.Type,
		Payload: newPayload,
	}

	g.mu.Lock()
	for _, p := range g.players {
		select {
		case p.send <- final:
		default:
			fmt.Println("dropping message for", p.id)
		}
	}
	g.mu.Unlock()
}
