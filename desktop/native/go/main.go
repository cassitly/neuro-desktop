package main

import (
	"fmt"
	"log"

	""
)

func main() {
	client, err := neuro.NewClient("", "ws://localhost:8080/neuro")
	if err != nil {
		log.Fatal(err)
	}

	defer client.Close()

	if err := client.Startup(); err != nil {
		log.Fatal(err)
	}

	client.SendContext("Game started!", true)

	// register actions your game supports
	client.RegisterActions([]map[string]interface{}{
		{"name":"move","description":"Move someplace"},
		{"name":"attack","description":"Attack the enemy"},
	})

	for {
		select {
		case act := <-client.ActionChan:
			fmt.Printf("Got action: %s\n", act.Name)

			// validate + execute in your game here...
			client.SendActionResult(act.ID, true, "ok")
		case err := <-client.ErrChan:
			log.Fatal("Neuro error:", err)
		}
	}
}
