package main

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

const maxPlayers = 8

type player struct {
	id   string
	name string
	conn *websocket.Conn
	send chan message
}

type message struct {
	message []byte
	p       *player
}

func newPlayer(id string, name string, conn *websocket.Conn) *player {
	return &player{
		id:   id,
		name: name,
		conn: conn,
		send: make(chan message),
	}
}

type game struct {
	slug          string
	players       map[string]*player
	mu            sync.Mutex
	broadcast     chan message
	currentPlayer *player
	currentWord   string
}

func newGame(slug string) *game {
	return &game{
		slug:      slug,
		players:   make(map[string]*player),
		broadcast: make(chan message),
	}
}

func (g *game) addPlayer(p *player) {
	fmt.Println("new player", p)
	g.mu.Lock()
	g.players[p.id] = p
	g.mu.Unlock()
	go p.readLoop(g)
	go p.writeLoop()
}

func (p *player) readLoop(g *game) {
	defer func() {
		p.conn.Close()
	}()

	for {
		_, msg, err := p.conn.ReadMessage()
		if err != nil {
			return
		}

		g.broadcast <- message{
			message: msg,
			p:       p,
		}
	}
}

func (p *player) writeLoop() {
	for msg := range p.send {
		err := p.conn.WriteMessage(websocket.TextMessage, msg.message)
		if err != nil {
			return
		}
	}
}

func (g *game) numPlayers() int {
	return len(g.players)
}

func (g *game) begin() {
	for _, v := range g.players {
		g.currentPlayer = v
	}
	g.currentWord = "sea"

	for {
		select {
		case msg := <-g.broadcast:
			{
				g.mu.Lock()
				for _, p := range g.players {
					p.send <- msg
				}
			}
		}
	}
}
