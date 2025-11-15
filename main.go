package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	activeGames := make(map[string]*game)
	players := make(map[string]*player)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Read error:", err)
			break
		}
		var jsonMsg Message
		if err := json.Unmarshal(msg, &jsonMsg); err != nil {
			fmt.Println("Invalid JSON:", err)
			continue
		}

		if jsonMsg.Type == "join" {
			id := jsonMsg.Payload["id"].(string)
			name := jsonMsg.Payload["name"].(string)
			if _, ok := players[id]; ok == false {
				p := newPlayer(id, name, conn)
				players[id] = p
				if len(activeGames) == 0 {
					g := newGame("game")
					activeGames[g.slug] = g
				}
				found := false
				for _, v := range activeGames {
					if v.numPlayers() < 8 {
						v.addPlayer(p)
						if v.numPlayers() == 8 {
							v.begin()
						}
						found = true
						break
					}
				}
				if !found {
					g := newGame("game")
					activeGames[g.slug] = g
					g.addPlayer(p)
				}
			}
		}
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	http.ListenAndServe(":8000", nil)
}
