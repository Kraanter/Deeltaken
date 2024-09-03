package utility

import (
	"fmt"

	"github.com/gorilla/websocket"
)

func BindInputChannelToWebsocket(conn *websocket.Conn, channel chan string, stop func()) {
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Could not read message", err)
				stop()
				return
			}

			println("Received message from websocket", string(message))

			channel <- string(message)
		}
	}()
}

func BindWebsocketToOutputChannel(conn *websocket.Conn, channel chan string) {
	for {
		message, ok := <-channel
		if !ok {
			return
		}
		println("Sending message to websocket", message)
		err := conn.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			fmt.Println("Could not write message", err)
			return
		}
	}
}
