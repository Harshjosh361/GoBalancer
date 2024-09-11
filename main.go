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
	"time"
)

// Load-balancer
type LoadBalancer struct {
	Current int // current server index
	Mutex   sync.Mutex
}

// Server
type Server struct {
	URL       *url.URL
	isHealthy bool
	Mutex     sync.Mutex
}

type Config struct {
	Port                string   `json:"port"`
	HealthCheckInterval string   `json:"healthCheckInterval"`
	Servers             []string `json:"servers"`
}

func main() {
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatal("Error loading configuration")
	}

	healthCheckInterval, err := time.ParseDuration(config.HealthCheckInterval)
	if err != nil {
		log.Fatal("Invalid health check interval")
	}

	var servers []*Server
	for _, serverURL := range config.Servers {
		u, _ := url.Parse(serverURL)
		server := &Server{URL: u, isHealthy: true}
		servers = append(servers, server)
		go CheckHealth(server, healthCheckInterval)
	}

	lb := LoadBalancer{Current: 0}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server := lb.getNextServer(servers)
		if server == nil {
			http.Error(w, "no healthy server available", http.StatusServiceUnavailable)
			return
		}
		w.Header().Add("X-Forwarded Server", server.URL.String())
		server.ReverseProxy().ServeHTTP(w, r)

	})
	log.Println("Starting load balancer on port", config.Port)
	err = http.ListenAndServe(config.Port, nil)
	if err != nil {
		log.Fatalf("Error starting load balancer: %s\n", err.Error())
	}
}

// load balancer algorithm
func (lb *LoadBalancer) getNextServer(servers []*Server) *Server {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	// loop to find healthy server
	for i := 0; i < len(servers); i++ {
		server := servers[lb.Current]
		lb.Current = (lb.Current + 1) % len(servers)

		// check if server is healthy
		server.Mutex.Lock()
		if server.isHealthy {
			server.Mutex.Unlock()
			return server
		}
		server.Mutex.Unlock()

	}
	return nil

}

func CheckHealth(s *Server, healthCheckInterval time.Duration) {
	for range time.Tick(healthCheckInterval) {
		// head request  to server
		res, err := http.Head(s.URL.String())

		s.Mutex.Lock()
		if err != nil || res.StatusCode != http.StatusOK {
			fmt.Printf("%s is down\n", s.URL)
			s.isHealthy = false
		} else {
			s.isHealthy = true
		}
		s.Mutex.Unlock()
		// close the response body using condn [runtime error if res is  nil as we cant access it]
		if res != nil {
			res.Body.Close()
		}

	}
}

func (s *Server) ReverseProxy() *httputil.ReverseProxy {
	return httputil.NewSingleHostReverseProxy(s.URL)
}

func loadConfig(file string) (Config, error) {
	var config Config
	data, err := os.ReadFile(file)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}
