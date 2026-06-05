package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var titles = []string{
	"Buy groceries",
	"Fix the CI pipeline",
	"Write unit tests",
	"Review pull request",
	"Update documentation",
	"Deploy to production",
	"Refactor auth module",
	"Add rate limiting",
	"Profile memory usage",
	"Rotate secrets",
}

func randomTitle() string {
	return titles[rand.Intn(len(titles))]
}

func get(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}
	return nil
}

func post(client *http.Client, url string, body any) error {
	b, _ := json.Marshal(body)
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("request error: %d", resp.StatusCode)
	}
	return nil
}

func main() {
	baseURL := os.Getenv("DEMO_URL")
	if baseURL == "" {
		baseURL = "http://demoapp:9000"
	}

	client := &http.Client{Timeout: 10 * time.Second}

	log.Printf("load generator starting, target: %s", baseURL)

	// Wait for the demo app to be ready before starting load.
	for {
		if err := get(client, baseURL+"/healthz"); err == nil {
			log.Printf("demo app is ready, starting load generation")
			break
		}
		log.Printf("waiting for demo app to be ready...")
		time.Sleep(2 * time.Second)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ok, fail := 0, 0

		// 6x GET /tasks
		for range 6 {
			if err := get(client, baseURL+"/tasks"); err != nil {
				fail++
			} else {
				ok++
			}
		}

		// 2x POST /tasks
		for range 2 {
			if err := post(client, baseURL+"/tasks", map[string]string{"title": randomTitle()}); err != nil {
				fail++
			} else {
				ok++
			}
		}

		// 1x GET /slow
		if err := get(client, baseURL+"/slow"); err != nil {
			fail++
		} else {
			ok++
		}

		// 1x GET /cpu
		if err := get(client, baseURL+"/cpu"); err != nil {
			fail++
		} else {
			ok++
		}

		log.Printf("batch complete: ok=%d fail=%d", ok, fail)
	}
}
