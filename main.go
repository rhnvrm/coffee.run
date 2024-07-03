package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/mux"
	"golang.org/x/exp/rand"
)

//go:embed page.html
var pageHTML string

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

type Menu struct {
	Items map[string]*MenuItem `json:"items"`
	sync.RWMutex
}

func newMenu() *Menu {
	return &Menu{
		Items: make(map[string]*MenuItem),
	}
}

type Session struct {
	URI  string `json:"uri"`
	Menu *Menu  `json:"menu"`
}

var sessions = make(map[string]*Session)

func newSession(url string) *Session {
	s := &Session{
		URI:  url,
		Menu: newMenu(),
	}
	sessions[url] = s
	return s
}

func loadEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/new", handleNewSession)
	r.HandleFunc("/api/{session}/menu", handleGetMenu)
	r.HandleFunc("/api/{session}/update", handleUpdate)

	r.HandleFunc("/", serveIndex)
	r.HandleFunc("/session/{session}", servePage)

	address := loadEnv("HTTP_ADDRESS", ":8080")
	log.Printf("Server started on http://%s", address)
	log.Fatal(http.ListenAndServe(address, r))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(indexHTML))
}

func servePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(pageHTML))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func handleNewSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	sess := &Session{
		URI:  randomString(8),
		Menu: newMenu(),
	}

	json.NewEncoder(w).Encode(Envelope{Status: "ok", Data: sess})
}

func handleGetMenu(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	session, ok := vars["session"]
	if !ok {
		http.Error(w, "Session is required", http.StatusBadRequest)
		return
	}

	sess, ok := sessions[session]
	if !ok {
		sess = newSession(session)
	}

	sess.Menu.RLock()
	defer sess.Menu.RUnlock()
	json.NewEncoder(w).Encode(Envelope{Status: "ok", Data: sess.Menu.Items})
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	session, ok := vars["session"]
	if !ok {
		http.Error(w, "Session is required", http.StatusBadRequest)
		return
	}

	sess, ok := sessions[session]
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
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

	sess.Menu.Lock()
	defer sess.Menu.Unlock()

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

		if _, ok := sess.Menu.Items[req.Item]; ok {
			http.Error(w, "Item already exists", http.StatusBadRequest)
			return
		}

		sess.Menu.Items[req.Item] = &MenuItem{
			Name:       req.Item,
			Count:      0,
			OwnerCount: map[string]int{},
		}

	case "remove":
		delete(sess.Menu.Items, req.Item)

	case "increment":
		var data struct {
			Owner string `json:"owner"`
		}

		if err := json.Unmarshal(req.Data, &data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if item, ok := sess.Menu.Items[req.Item]; ok {
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

		if item, ok := sess.Menu.Items[req.Item]; ok {
			if item.Count > 0 {
				item.Count--
				item.OwnerCount[data.Owner]--
			}
		} else {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(Envelope{Status: "ok", Data: sess.Menu.Items})
}
