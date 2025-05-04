package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
)

type LoadBalancer struct {
	Current int
	Mutex   sync.Mutex
}

type Server struct {
	URL       *url.URL
	IsHealthy bool
	Mutex     sync.Mutex
}

type Config struct {
	Port    string   `json:"port"`
	Servers []string `json:"servers"`
}

var config Config

func init() {
	file, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(file, &config)
	if err != nil {
		log.Fatal(err)
	}
}

func (lb *LoadBalancer) getNextServer(servers []*Server) *Server {
	log.Printf("Next server requested...")
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	for i := 0; i < len(servers); i++ {
		idx := lb.Current % len(servers)
		nextServer := servers[idx]
		lb.Current++

		healthCheck(nextServer)
		log.Printf("Health Check!")
		isHealthy := nextServer.IsHealthy

		if isHealthy {
			return nextServer
		}
	}

	return nil
}

func healthCheck(s *Server) {

	res, err := http.Head(s.URL.String())

	s.Mutex.Lock()

	if err != nil || res.StatusCode != http.StatusOK {
		fmt.Printf("%s is down\n", s.URL)
		s.IsHealthy = false
	} else {
		s.IsHealthy = true
	}

	s.Mutex.Unlock()

}

func (s *Server) ReverseProxy() *httputil.ReverseProxy {
	return httputil.NewSingleHostReverseProxy(s.URL)
}

func main() {
	log.Println("Starting...")

	cache := make(map[string]*Server)

	servers := []*Server{}

	for _, server := range config.Servers {
		if parsedURL, err := url.Parse(server); err == nil {
			servers = append(servers, &Server{URL: parsedURL, IsHealthy: true})
		}
	}

	lb := LoadBalancer{Current: 0}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var server *Server
		if cache[r.URL.String()] != nil {
			server = cache[r.URL.String()]
			log.Println("ip found")
		} else {
			log.Println("New ip")
			server = lb.getNextServer(servers)
			cache[r.URL.String()] = server
		}

		if server == nil {
			http.Error(w, "No healthy server available", http.StatusServiceUnavailable)
			return
		}
		w.Header().Add("Forwarded-Server", server.URL.String())
		server.ReverseProxy().ServeHTTP(w, r)
	})

	err := http.ListenAndServe(config.Port, nil)
	log.Printf("Listening on port %s\n", config.Port)
	if err != nil {
		log.Fatalf("Error starting load balancer: %s\n", err.Error())
	}
}
