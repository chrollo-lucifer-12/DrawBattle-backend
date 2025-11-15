package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var activeGames map[string]*game
var players map[string]*player

func wsHandler(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Upgrade error:", err)
		return
	}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Read error:", err)
			return
		}

		var jsonMsg Message
		if err := json.Unmarshal(msg, &jsonMsg); err != nil {
			fmt.Println("Invalid JSON:", err)
			continue
		}
		if jsonMsg.Type == "join" {

			id, _ := jsonMsg.Payload["id"].(string)
			name, _ := jsonMsg.Payload["name"].(string)

			if id == "" || name == "" {
				fmt.Println("Invalid join payload")
				continue
			}

			if _, ok := players[id]; ok {
				fmt.Println("Player already exists:", id)
				continue
			}

			p := newPlayer(id, name, conn)
			players[id] = p

			if len(activeGames) == 0 {
				g := newGame("game")
				activeGames[g.slug] = g
			}
			assigned := false
			for _, g := range activeGames {
				if g.numPlayers() < maxPlayers {
					g.addPlayer(p)
					if g.numPlayers() == maxPlayers {
						go g.begin()
					}

					assigned = true
					break
				}
			}
			if !assigned {
				g := newGame("game")
				activeGames[g.slug] = g
				g.addPlayer(p)
			}
			return
		}
	}
}

func main() {
	activeGames = make(map[string]*game)
	players = make(map[string]*player)

	http.HandleFunc("/ws", wsHandler)
	http.ListenAndServe(":8000", nil)
}
