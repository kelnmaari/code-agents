package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// Server is the observability HTTP server that exposes:
//   - GET /         — real-time web dashboard (HTML, polls /status)
//   - GET /metrics  — Prometheus text format
//   - GET /status   — JSON snapshot
type Server struct {
	collector *Collector
	srv       *http.Server
}

// NewServer creates a Server bound to the given port.
func NewServer(collector *Collector, port int) *Server {
	s := &Server{collector: collector}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/", s.handleUI)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	return s
}

// Start begins serving in a background goroutine.
// Returns the actual bound address (useful when port == 0).
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return "", fmt.Errorf("metrics server listen: %w", err)
	}
	go func() { _ = s.srv.Serve(ln) }()
	return ln.Addr().String(), nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// handleMetrics writes Prometheus-format metrics text.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	snap := s.collector.Snapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintf(w, "# HELP tasks_pending Number of tasks currently in pending state\n")
	fmt.Fprintf(w, "# TYPE tasks_pending gauge\n")
	fmt.Fprintf(w, "tasks_pending %d\n\n", snap.TasksPending)

	fmt.Fprintf(w, "# HELP tasks_completed_total Total number of successfully completed tasks\n")
	fmt.Fprintf(w, "# TYPE tasks_completed_total counter\n")
	fmt.Fprintf(w, "tasks_completed_total %d\n\n", snap.TasksCompleted)

	fmt.Fprintf(w, "# HELP tasks_failed_total Total number of failed tasks\n")
	fmt.Fprintf(w, "# TYPE tasks_failed_total counter\n")
	fmt.Fprintf(w, "tasks_failed_total %d\n\n", snap.TasksFailed)

	fmt.Fprintf(w, "# HELP tokens_used_total Total tokens consumed (prompt + completion)\n")
	fmt.Fprintf(w, "# TYPE tokens_used_total counter\n")
	fmt.Fprintf(w, "tokens_used_total %d\n", snap.TokensTotal)
	fmt.Fprintf(w, "tokens_used_prompt_total %d\n", snap.TokensPrompt)
	fmt.Fprintf(w, "tokens_used_completion_total %d\n\n", snap.TokensCompletion)

	fmt.Fprintf(w, "# HELP worker_busy_ratio Fraction of workers currently executing a task (0..1)\n")
	fmt.Fprintf(w, "# TYPE worker_busy_ratio gauge\n")
	fmt.Fprintf(w, "worker_busy_ratio %.4f\n\n", snap.WorkerBusyRatio)

	fmt.Fprintf(w, "# HELP queue_wait_time_avg_seconds Rolling average time a task waits in the queue before being picked up\n")
	fmt.Fprintf(w, "# TYPE queue_wait_time_avg_seconds gauge\n")
	fmt.Fprintf(w, "queue_wait_time_avg_seconds %.4f\n\n", snap.QueueWaitAvgSec)

	fmt.Fprintf(w, "# HELP uptime_seconds Seconds elapsed since the agent process started\n")
	fmt.Fprintf(w, "# TYPE uptime_seconds counter\n")
	fmt.Fprintf(w, "uptime_seconds %.2f\n", snap.UptimeSeconds)
}

// handleStatus returns a JSON snapshot of all current metrics.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	snap := s.collector.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(snap)
}

// handleUI serves the built-in single-page dashboard.
func (s *Server) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, dashboardHTML)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
<title>Code-Agents Dashboard</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:'Segoe UI',system-ui,sans-serif;background:#0f1117;color:#e2e8f0;min-height:100vh;padding:24px}
  h1{font-size:1.6rem;font-weight:700;margin-bottom:4px;color:#7dd3fc}
  .subtitle{color:#64748b;font-size:.875rem;margin-bottom:28px}
  .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:16px;margin-bottom:28px}
  .card{background:#1e2330;border-radius:12px;padding:20px;border:1px solid #2d3447}
  .card-label{font-size:.75rem;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:#64748b;margin-bottom:8px}
  .card-value{font-size:2rem;font-weight:700;line-height:1}
  .card-sub{font-size:.75rem;color:#94a3b8;margin-top:6px}
  .pending{color:#fbbf24}
  .completed{color:#34d399}
  .failed{color:#f87171}
  .tokens{color:#a78bfa}
  .busy{color:#60a5fa}
  .wait{color:#fb923c}
  .bar-wrap{background:#2d3447;border-radius:999px;height:10px;margin-top:10px;overflow:hidden}
  .bar{height:100%;border-radius:999px;transition:width .4s ease;background:#60a5fa}
  .footer{color:#475569;font-size:.75rem;text-align:right}
  .dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:#34d399;margin-right:6px;animation:pulse 2s infinite}
  @keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
  .error-banner{background:#450a0a;border:1px solid #991b1b;border-radius:8px;padding:12px 16px;color:#fca5a5;margin-bottom:20px;display:none}
</style>
</head>
<body>
<h1>&#128200; Code-Agents Dashboard</h1>
<p class="subtitle"><span class="dot"></span>Live metrics &mdash; refreshes every 2 seconds</p>
<div class="error-banner" id="err">&#9888; Cannot reach /status. Agent may have stopped.</div>

<div class="grid">
  <div class="card">
    <div class="card-label">Tasks Pending</div>
    <div class="card-value pending" id="pending">–</div>
    <div class="card-sub">in queue</div>
  </div>
  <div class="card">
    <div class="card-label">Tasks Completed</div>
    <div class="card-value completed" id="completed">–</div>
    <div class="card-sub">all time</div>
  </div>
  <div class="card">
    <div class="card-label">Tasks Failed</div>
    <div class="card-value failed" id="failed">–</div>
    <div class="card-sub">all time</div>
  </div>
  <div class="card">
    <div class="card-label">Tokens Used</div>
    <div class="card-value tokens" id="tokens">–</div>
    <div class="card-sub" id="tokens-sub">prompt / completion</div>
  </div>
  <div class="card">
    <div class="card-label">Worker Busy Ratio</div>
    <div class="card-value busy" id="busy-ratio">–</div>
    <div class="card-sub" id="busy-sub">workers active</div>
    <div class="bar-wrap"><div class="bar" id="busy-bar" style="width:0%"></div></div>
  </div>
  <div class="card">
    <div class="card-label">Avg Queue Wait</div>
    <div class="card-value wait" id="wait">–</div>
    <div class="card-sub">seconds (rolling avg)</div>
  </div>
  <div class="card">
    <div class="card-label">Uptime</div>
    <div class="card-value" id="uptime" style="color:#94a3b8">–</div>
    <div class="card-sub">hh:mm:ss</div>
  </div>
</div>
<p class="footer" id="last-update">Waiting for first update&hellip;</p>

<script>
function fmt(n){return n==null?'–':n.toLocaleString()}
function fmtRatio(r){return r==null?'–':(r*100).toFixed(1)+'%'}
function fmtWait(s){return s==null?'–':s.toFixed(2)+'s'}
function fmtUptime(s){
  if(s==null)return'–';
  const h=Math.floor(s/3600),m=Math.floor((s%3600)/60),sec=Math.floor(s%60);
  return[h,m,sec].map(v=>String(v).padStart(2,'0')).join(':');
}
async function poll(){
  try{
    const r=await fetch('/status');
    if(!r.ok)throw new Error('status '+r.status);
    const d=await r.json();
    document.getElementById('err').style.display='none';
    document.getElementById('pending').textContent=fmt(d.tasks_pending);
    document.getElementById('completed').textContent=fmt(d.tasks_completed);
    document.getElementById('failed').textContent=fmt(d.tasks_failed);
    document.getElementById('tokens').textContent=fmt(d.tokens_total);
    document.getElementById('tokens-sub').textContent=
      'prompt '+fmt(d.tokens_prompt)+' / compl '+fmt(d.tokens_completion);
    const pct=(d.worker_busy_ratio*100).toFixed(1);
    document.getElementById('busy-ratio').textContent=fmtRatio(d.worker_busy_ratio);
    document.getElementById('busy-sub').textContent=
      d.worker_busy+' of '+d.worker_total+' busy';
    document.getElementById('busy-bar').style.width=pct+'%';
    document.getElementById('wait').textContent=fmtWait(d.queue_wait_time_avg_sec);
    document.getElementById('uptime').textContent=fmtUptime(d.uptime_seconds);
    document.getElementById('last-update').textContent=
      'Last update: '+new Date().toLocaleTimeString();
  }catch(e){
    document.getElementById('err').style.display='block';
  }
}
poll();
setInterval(poll,2000);
</script>
</body>
</html>`
