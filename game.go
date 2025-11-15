package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

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
	go func() {
		for {

			g.nextPlayer()

			g.mu.Lock()
			drawer := g.currentPlayer
			g.currentWord = ""
			g.mu.Unlock()

			if drawer == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			fmt.Println("Turn for", drawer.id)

			drawer.send <- Message{
				Type: "choose_word",
			}

			wordChosen := make(chan struct{})

			go func() {
				for {
					g.mu.Lock()
					hasWord := g.currentWord != ""
					g.mu.Unlock()

					if hasWord {
						close(wordChosen)
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			}()

			select {
			case <-wordChosen:
				fmt.Println("Word chosen:", g.currentWord)
			case <-time.After(20 * time.Second):

				fmt.Println("Drawer took too long, skipping...")
				continue
			}

			time.Sleep(80 * time.Second)

			g.mu.Lock()
			word := g.currentWord
			drawer = g.currentPlayer
			g.mu.Unlock()

			g.broadcastMessage(gameMessage{
				Message: Message{
					Type: "reveal_word",
					Payload: map[string]any{
						"word": word,
						"id":   drawer.id,
					},
				},
			})

			time.Sleep(10 * time.Second)
		}
	}()

	for {
		msg := <-g.broadcast

		switch msg.Type {

		case "word":
			g.mu.Lock()
			isDrawer := g.currentPlayer != nil && msg.p != nil && msg.p.id == g.currentPlayer.id
			g.mu.Unlock()

			if isDrawer {
				g.currentWord = msg.Payload["word"].(string)
			}

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
				g.broadcastMessage(gameMessage{
					Message: Message{
						Type: "chat",
						Payload: map[string]any{
							"sender":  msg.p.id,
							"message": raw,
						},
					},
				})
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

func (g *game) nextPlayer() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.players) == 0 {
		g.currentPlayer = nil
		return
	}

	playerList := make([]*player, 0, len(g.players))
	for _, p := range g.players {
		playerList = append(playerList, p)
	}

	if g.currentPlayer == nil {
		g.currentPlayer = playerList[0]
		return
	}

	var idx int
	for i, p := range playerList {
		if p.id == g.currentPlayer.id {
			idx = i
			break
		}
	}

	g.currentPlayer = playerList[(idx+1)%len(playerList)]

	fmt.Println("turn ", g.currentPlayer.id)
}
