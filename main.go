package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
)

//go:embed index.html
var indexHTML string

type Envelope struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type MenuItem struct {
	Name       string         `json:"name"`
	OwnerCount map[string]int `json:"owner_count"`
	Count      int            `json:"count"`
}

var menu = struct {
	sync.RWMutex
	Items map[string]*MenuItem `json:"items"`
}{Items: make(map[string]*MenuItem)}

func loadEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	http.HandleFunc("/menu", handleMenu)
	http.HandleFunc("/update", handleUpdate)

	http.HandleFunc("/", serveIndex)

	address := loadEnv("HTTP_ADDRESS", ":8080")
	log.Println("Server started on ", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(indexHTML))
}

func handleMenu(w http.ResponseWriter, r *http.Request) {
	menu.RLock()
	defer menu.RUnlock()
	json.NewEncoder(w).Encode(Envelope{Status: "ok", Data: menu.Items})
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Item   string          `json:"item"`
		Action string          `json:"action"` // "add", "remove", "increment", "decrement"
		Data   json.RawMessage `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	menu.Lock()
	defer menu.Unlock()

	switch req.Action {
	case "add":
		var data struct {
			Name string `json:"name"`
		}

		if err := json.Unmarshal(req.Data, &data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if data.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		if _, ok := menu.Items[req.Item]; ok {
			http.Error(w, "Item already exists", http.StatusBadRequest)
			return
		}

		menu.Items[req.Item] = &MenuItem{
			Name:       req.Item,
			Count:      0,
			OwnerCount: map[string]int{},
		}

	case "remove":
		delete(menu.Items, req.Item)

	case "increment":
		var data struct {
			Owner string `json:"owner"`
		}

		if err := json.Unmarshal(req.Data, &data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if item, ok := menu.Items[req.Item]; ok {
			item.Count++
			if v, ok := item.OwnerCount[data.Owner]; ok {
				item.OwnerCount[data.Owner] = v + 1
			} else {
				item.OwnerCount[data.Owner] = 1
			}
		} else {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
	case "decrement":
		var data struct {
			Owner string `json:"owner"`
		}

		if err := json.Unmarshal(req.Data, &data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if item, ok := menu.Items[req.Item]; ok {
			if item.Count > 0 {
				item.Count--
				item.OwnerCount[data.Owner]--
				if item.OwnerCount[data.Owner] == 0 {
					delete(item.OwnerCount, data.Owner)
				}
			}
		} else {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(Envelope{Status: "ok", Data: menu.Items})
}
