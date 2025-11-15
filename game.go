package main

import "github.com/gorilla/websocket"

const maxPlayers = 8

type player struct {
	id   string
	name string
	conn *websocket.Conn
}

func newPlayer(id string, name string, conn *websocket.Conn) *player {
	return &player{
		id:   id,
		name: name,
		conn: conn,
	}
}

type game struct {
	slug    string
	players []player
}

func newGame(slug string) *game {
	return &game{
		slug: slug,
	}
}

func (g *game) addPlayer(p player) {
	g.players = append(g.players, p)
}

func (g *game) removePlayer(id string) {
	out := []player{}
	for _, p := range g.players {
		if p.id != id {
			out = append(out, p)
		}
	}
	g.players = out
}
