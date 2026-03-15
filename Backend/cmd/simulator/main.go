// cmd/simulator/main.go
//
// BehaviourLens load simulator.
// Spawns N virtual users, each running a realistic browsing journey and posting
// events to the backend.  Designed for local dev and demo purposes.
//
// Usage:
//
//	go run ./cmd/simulator -url http://localhost:8080 -users 10 -rate 400
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ── CLI flags ─────────────────────────────────────────────────────────────────

var (
	backendURL = flag.String("url", "http://localhost:8080", "Backend base URL")
	numUsers   = flag.Int("users", 10, "Number of concurrent virtual users")
	rateMs     = flag.Int("rate", 500, "Average milliseconds between events per user")
	duration   = flag.Duration("duration", 0, "How long to run (0 = run forever)")
)

// ── event payload ─────────────────────────────────────────────────────────────

type eventPayload struct {
	UserID    string            `json:"user_id"`
	Action    string            `json:"action"`
	Page      string            `json:"page"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ── page topology ─────────────────────────────────────────────────────────────

// pages represents all named pages the simulator can visit.
var pages = []string{
	"/home",
	"/products",
	"/product/detail",
	"/search",
	"/cart",
	"/checkout",
	"/payment",
	"/confirm",
	"/about",
}

// checkoutFlow defines the checkout funnel pages in order.
var checkoutFlow = []string{"/cart", "/checkout", "/payment"}

// ── virtual user ──────────────────────────────────────────────────────────────

// virtualUser drives one user's session.
// It chooses a scenario at random and executes it in a loop.
type virtualUser struct {
	id     string
	client *http.Client
	rng    *rand.Rand
	baseMS int // base inter-event delay in ms
}

func newVirtualUser(n int, baseMS int) *virtualUser {
	// Each user gets an independently seeded RNG so parallel runs differ.
	return &virtualUser{
		id:     fmt.Sprintf("usr_%04d", n),
		client: &http.Client{Timeout: 5 * time.Second},
		rng:    rand.New(rand.NewSource(time.Now().UnixNano() + int64(n)*997)),
		baseMS: baseMS,
	}
}

func (u *virtualUser) run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		// Pick a random scenario and run it.
		scenario := u.rng.Intn(5)
		switch scenario {
		case 0:
			u.scenarioNormalBrowse()
		case 1:
			u.scenarioHesitation()
		case 2:
			u.scenarioNavigationLoop()
		case 3:
			u.scenarioCheckoutAbandonment()
		case 4:
			u.scenarioFullPurchase()
		}

		// Rest briefly between sessions.
		u.sleep(2000, 5000)
	}
}

// ── scenario implementations ──────────────────────────────────────────────────

// scenarioNormalBrowse visits a few random pages without any friction.
func (u *virtualUser) scenarioNormalBrowse() {
	n := 2 + u.rng.Intn(4)
	for i := 0; i < n; i++ {
		page := pages[u.rng.Intn(len(pages))]
		u.post("navigate", page, nil)
		u.sleep(300, u.baseMS*2)
		if u.rng.Intn(3) == 0 {
			u.post("scroll", page, map[string]string{"depth": fmt.Sprintf("%d", 20+u.rng.Intn(80))})
			u.sleep(200, 600)
		}
		if u.rng.Intn(4) == 0 {
			u.post("click", page, nil)
			u.sleep(100, 400)
		}
	}
}

// scenarioHesitation pauses on /checkout or /pricing for a long idle period.
func (u *virtualUser) scenarioHesitation() {
	hesitationPages := []string{"/checkout", "/payment", "/products", "/product/detail"}
	page := hesitationPages[u.rng.Intn(len(hesitationPages))]

	u.post("navigate", page, nil)
	u.sleep(200, 500)

	// Idle for 10–45 seconds — triggers low/medium/high hesitation depending on duration.
	durationMs := 10_000 + u.rng.Intn(35_000)
	u.post("idle", page, map[string]string{
		"duration_ms": fmt.Sprintf("%d", durationMs),
	})
	// Actual sleep is shorter so the simulator doesn't stall — the metadata carries the signal.
	u.sleep(500, 1000)

	// After hesitation, user may bounce.
	if u.rng.Intn(2) == 0 {
		u.post("navigate", "/home", nil)
	}
}

// scenarioNavigationLoop bounces the user between two pages several times.
func (u *virtualUser) scenarioNavigationLoop() {
	loopPairs := [][2]string{
		{"/search", "/products"},
		{"/cart", "/checkout"},
		{"/product/detail", "/search"},
	}
	pair := loopPairs[u.rng.Intn(len(loopPairs))]
	visits := 3 + u.rng.Intn(3) // 3–5 visits to trigger low/medium/high

	for i := 0; i < visits; i++ {
		page := pair[i%2]
		u.post("navigate", page, nil)
		u.sleep(200, u.baseMS)
		if u.rng.Intn(2) == 0 {
			u.post("click", page, nil)
			u.sleep(100, 300)
		}
	}
}

// scenarioCheckoutAbandonment walks into the checkout funnel and bails out.
func (u *virtualUser) scenarioCheckoutAbandonment() {
	// Enter funnel from products.
	u.post("navigate", "/products", nil)
	u.sleep(300, 800)
	u.post("click", "/products", nil)
	u.sleep(200, 400)

	// Walk through part of the checkout flow.
	depth := 1 + u.rng.Intn(len(checkoutFlow)) // 1–3 steps
	for i := 0; i < depth; i++ {
		u.post("navigate", checkoutFlow[i], nil)
		u.sleep(300, u.baseMS)
	}

	// Optional hesitation before bailing.
	if u.rng.Intn(2) == 0 {
		lastPage := checkoutFlow[depth-1]
		idleMs := 5_000 + u.rng.Intn(20_000)
		u.post("idle", lastPage, map[string]string{
			"duration_ms": fmt.Sprintf("%d", idleMs),
		})
		u.sleep(300, 600)
	}

	// Abandon: navigate away or fire explicit abandon event.
	if u.rng.Intn(2) == 0 {
		u.post("abandon", checkoutFlow[depth-1], nil)
	} else {
		u.post("navigate", "/home", nil)
	}
}

// scenarioFullPurchase completes the entire checkout flow successfully.
func (u *virtualUser) scenarioFullPurchase() {
	u.post("navigate", "/products", nil)
	u.sleep(200, 600)
	u.post("click", "/products", nil)
	u.sleep(200, 400)

	for _, page := range checkoutFlow {
		u.post("navigate", page, nil)
		u.sleep(300, u.baseMS)
		u.post("click", page, nil)
		u.sleep(100, 300)
	}

	u.post("purchase", "/confirm", nil)
	u.sleep(100, 200)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (u *virtualUser) post(action, page string, meta map[string]string) {
	e := eventPayload{
		UserID:    u.id,
		Action:    action,
		Page:      page,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  meta,
	}

	body, err := json.Marshal(e)
	if err != nil {
		log.Printf("[%s] marshal error: %v", u.id, err)
		return
	}

	resp, err := u.client.Post(*backendURL+"/events", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[%s] POST error: %v", u.id, err)
		return
	}
	resp.Body.Close()

	log.Printf("[%s] %s %-20s → %d", u.id, action, page, resp.StatusCode)
}

// sleep pauses for a random duration between minMs and minMs+spreadMs milliseconds.
func (u *virtualUser) sleep(minMs, spreadMs int) {
	ms := minMs + u.rng.Intn(spreadMs+1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	log.Printf("BehaviourLens Simulator starting: users=%d rate=%dms url=%s", *numUsers, *rateMs, *backendURL)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 1; i <= *numUsers; i++ {
		wg.Add(1)
		user := newVirtualUser(i, *rateMs)
		go func() {
			defer wg.Done()
			user.run(stop)
		}()
	}

	if *duration > 0 {
		time.Sleep(*duration)
		close(stop)
		wg.Wait()
		log.Println("Simulator finished.")
	} else {
		// Block forever — Ctrl-C to stop.
		select {}
	}
}
