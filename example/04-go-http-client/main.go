// Pure Go MiniObserv HTTP client — stdlib only, no external packages.
//
// Reads MINIOBSERV_URL and AGENT_TOKEN from environment, mints a HS256 JWT
// manually using crypto/hmac, pushes a metric, then queries it back.
//
// Usage:
//
//	export MINIOBSERV_URL=http://localhost:8080
//	export AGENT_TOKEN=your-secret-here-min-16-chars
//	go run main.go
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ── JWT helpers (HS256, stdlib only) ──────────────────────────────────────────

func base64URLEncode(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.TrimRight(s, "=")
	return s
}

func jsonBase64URL(v any) string {
	b, _ := json.Marshal(v)
	return base64URLEncode(b)
}

// mintJWT creates a 24-hour HS256 JWT signed with the given secret.
func mintJWT(secret string) string {
	header  := jsonBase64URL(map[string]string{"alg": "HS256", "typ": "JWT"})
	now     := time.Now().Unix()
	payload := jsonBase64URL(map[string]any{
		"iss": "miniobserv-agent",
		"iat": now,
		"exp": now + 86400,
	})

	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig
}

// ── API types ─────────────────────────────────────────────────────────────────

type Metric struct {
	Time   string            `json:"time"`
	Host   string            `json:"host"`
	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}

type MetricBatch struct {
	Host    string   `json:"host"`
	Metrics []Metric `json:"metrics"`
}

type IngestResponse struct {
	Ingested int `json:"ingested"`
}

type QueryPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

type QueryResponse struct {
	Host   string       `json:"host"`
	Name   string       `json:"name"`
	Bucket string       `json:"bucket"`
	Agg    string       `json:"agg"`
	Points []QueryPoint `json:"points"`
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

type client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}


func (c *client) do(method, path string, body any, dst any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(raw))
	}

	if dst != nil {
		if err := json.Unmarshal(raw, dst); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
	}
	return nil
}

func (c *client) pushMetrics(batch MetricBatch) (*IngestResponse, error) {
	var resp IngestResponse
	if err := c.do("POST", "/api/v1/metrics", batch, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *client) queryMetrics(host, name string, from, to time.Time, bucket, agg string) (*QueryResponse, error) {
	params := url.Values{}
	params.Set("host",   host)
	params.Set("name",   name)
	params.Set("from",   from.UTC().Format(time.RFC3339))
	params.Set("to",     to.UTC().Format(time.RFC3339))
	params.Set("bucket", bucket)
	params.Set("agg",    agg)

	var resp QueryResponse
	if err := c.do("GET", "/api/v1/metrics/query?"+params.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── health ────────────────────────────────────────────────────────────────────

func checkHealth(baseURL string) error {
	resp, err := http.Get(baseURL + "/healthz") //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	baseURL    := os.Getenv("MINIOBSERV_URL")
	agentToken := os.Getenv("AGENT_TOKEN")

	if baseURL == "" {
		log.Fatal("MINIOBSERV_URL is not set")
	}
	if agentToken == "" {
		log.Fatal("AGENT_TOKEN is not set")
	}

	// ── 1. Health check ────────────────────────────────────────────────────────
	fmt.Println("── Step 1: Health check")
	if err := checkHealth(baseURL); err != nil {
		log.Fatalf("health check failed: %v", err)
	}
	fmt.Println("   Server is healthy")

	// ── 2. Mint JWT ────────────────────────────────────────────────────────────
	fmt.Println("── Step 2: Mint HS256 JWT")
	tok := mintJWT(agentToken)
	preview := tok
	if len(preview) > 60 {
		preview = preview[:60]
	}
	fmt.Printf("   %s...\n\n", preview)

	c := &client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      tok,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}

	// ── 3. Push a metric ───────────────────────────────────────────────────────
	fmt.Println("── Step 3: Push cpu.usage_pct metric")
	now := time.Now().UTC()
	batch := MetricBatch{
		Host: "go-example",
		Metrics: []Metric{
			{
				Time:  now.Format(time.RFC3339),
				Host:  "go-example",
				Name:  "cpu.usage_pct",
				Value: 37.8,
			},
			{
				Time:  now.Format(time.RFC3339),
				Host:  "go-example",
				Name:  "mem.used_pct",
				Value: 55.2,
			},
		},
	}

	ingestResp, err := c.pushMetrics(batch)
	if err != nil {
		log.Fatalf("push failed: %v", err)
	}
	fmt.Printf("   Ingested: %d metrics\n\n", ingestResp.Ingested)

	// ── 4. Query it back ───────────────────────────────────────────────────────
	fmt.Println("── Step 4: Query cpu.usage_pct (last 5 minutes)")
	queryResp, err := c.queryMetrics(
		"go-example",
		"cpu.usage_pct",
		now.Add(-5*time.Minute),
		now.Add(time.Minute), // +1m to catch the just-inserted point
		"1m",
		"avg",
	)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}

	fmt.Printf("   Host  : %s\n", queryResp.Host)
	fmt.Printf("   Metric: %s\n", queryResp.Name)
	fmt.Printf("   Bucket: %s   Agg: %s\n", queryResp.Bucket, queryResp.Agg)
	fmt.Printf("   Points: %d\n\n", len(queryResp.Points))

	if len(queryResp.Points) == 0 {
		fmt.Println("   (no points — TimescaleDB may need a moment to aggregate)")
	}
	for _, pt := range queryResp.Points {
		fmt.Printf("   %s  %.2f\n", pt.Time, pt.Value)
	}

	fmt.Println("\n── Done!")
}
