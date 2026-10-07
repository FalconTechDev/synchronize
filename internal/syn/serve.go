package syn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (e *Engine) Serve(cfg *Config, addr string) error {
	if addr == "" {
		addr = "127.0.0.1:7391"
		if cfg.ServeAddr != "" {
			addr = cfg.ServeAddr
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashboardHTML(cfg, e))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type row struct {
			Name     string `json:"name"`
			Branch   string `json:"branch"`
			Ahead    int    `json:"ahead"`
			Behind   int    `json:"behind"`
			Dirty    bool   `json:"dirty"`
			LastSync string `json:"last_sync"`
			Conflict bool   `json:"conflict"`
			Err      string `json:"error,omitempty"`
		}
		var rows []row
		for i := range cfg.Repos {
			repo := &cfg.Repos[i]
			st := e.Status(repo, false)
			ls := ""
			if !st.LastSync.IsZero() {
				ls = timeAgo(st.LastSync)
			}
			errStr := ""
			if st.Err != nil {
				errStr = st.Err.Error()
			}
			rows = append(rows, row{
				Name:     repo.Name,
				Branch:   st.Branch,
				Ahead:    st.Ahead,
				Behind:   st.Behind,
				Dirty:    st.Dirty,
				LastSync: ls,
				Conflict: st.Conflict != nil,
				Err:      errStr,
			})
		}
		json.NewEncoder(w).Encode(rows)
	})
	mux.HandleFunc("/api/pull", sameOriginOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		name := r.URL.Query().Get("repo")
		repo := cfg.Find(name)
		if repo == nil {
			http.Error(w, "repo not found", 404)
			return
		}
		res := e.Pull(repo)
		writeResult(w, res)
	}))
	mux.HandleFunc("/api/push", sameOriginOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		name := r.URL.Query().Get("repo")
		repo := cfg.Find(name)
		if repo == nil {
			http.Error(w, "repo not found", 404)
			return
		}
		res := e.Push(repo)
		writeResult(w, res)
	}))
	mux.HandleFunc("/api/log", func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", 500)
			return
		}
		stop := make(chan struct{})
		defer close(stop)
		pr, pw := newLineWriter(func(line string) {
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\n", " "))
			flusher.Flush()
		})
		go LiveTail(pw, stop)
		<-r.Context().Done()
		pr.Close()
	})

	if listensBeyondThisMachine(addr) {
		Warn("%s accepts connections from the network, and this dashboard can\n"+
			"       pull and push. Anyone who can reach it can publish your work.\n"+
			"       For a different port on this machine only:  --addr 127.0.0.1:8080", addr)
	}

	display := addr
	if strings.HasPrefix(addr, ":") {
		display = "localhost" + addr
	}
	Info("dashboard at http://%s/", display)
	return http.ListenAndServe(addr, mux)
}

func writeResult(w http.ResponseWriter, res Result) {
	w.Header().Set("Content-Type", "application/json")
	type out struct {
		Summary  string `json:"summary"`
		Error    string `json:"error,omitempty"`
		Conflict bool   `json:"conflict"`
	}
	o := out{Summary: res.Summary(), Conflict: res.Conflict != nil}
	if res.Err != nil {
		o.Error = res.Err.Error()
	}
	json.NewEncoder(w).Encode(o)
}

type lineWriter struct {
	ch   chan string
	done chan struct{}
}

func newLineWriter(fn func(string)) (*lineWriter, *lineWriter) {
	lw := &lineWriter{ch: make(chan string, 64), done: make(chan struct{})}
	go func() {
		for line := range lw.ch {
			fn(line)
		}
	}()
	return lw, lw
}

func (lw *lineWriter) Write(p []byte) (int, error) {
	select {
	case <-lw.done:
		return 0, fmt.Errorf("closed")
	case lw.ch <- string(p):
	}
	return len(p), nil
}

func (lw *lineWriter) Close() error {
	close(lw.done)
	close(lw.ch)
	return nil
}

func (lw *lineWriter) Read(p []byte) (int, error) { return 0, nil }

func dashboardHTML(cfg *Config, e *Engine) string {
	now := time.Now().Format("15:04:05")
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>synchronize</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#0f0f13;color:#e0e0e0;padding:24px}
h1{font-size:1.4rem;font-weight:600;margin-bottom:20px;color:#fff}
h1 span{color:#7c6df0;font-size:.9rem;font-weight:400;margin-left:8px}
.repos{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:16px;margin-bottom:28px}
.card{background:#18181f;border:1px solid #2a2a36;border-radius:12px;padding:18px}
.card-name{font-weight:600;font-size:1rem;margin-bottom:6px}
.card-meta{font-size:.78rem;color:#888;margin-bottom:12px}
.badge{display:inline-block;padding:2px 8px;border-radius:20px;font-size:.72rem;font-weight:600;margin-right:4px}
.ok{background:#1a3a2a;color:#4dde8a}
.conflict{background:#3a1a1a;color:#ff6b6b}
.dirty{background:#2a2a1a;color:#f0c040}
.btns{display:flex;gap:8px;margin-top:12px}
button{padding:6px 14px;border-radius:6px;border:none;cursor:pointer;font-size:.8rem;font-weight:600;transition:opacity .15s}
.btn-pull{background:#7c6df0;color:#fff}
.btn-push{background:#2a2a36;color:#aaa}
button:hover{opacity:.8}
.log-box{background:#18181f;border:1px solid #2a2a36;border-radius:12px;padding:16px;height:260px;overflow-y:auto;font-family:monospace;font-size:.75rem;line-height:1.5}
.log-box p{margin:0}
.log-err{color:#ff6b6b}.log-warn{color:#f0c040}.log-info{color:#4dde8a}.log-dbg{color:#666}
.footer{margin-top:10px;font-size:.72rem;color:#555}
</style>
</head>
<body>
<h1>synchronize <span>dashboard · ` + now + `</span></h1>
<div class="repos" id="repos">loading...</div>
<h2 style="margin-bottom:12px;font-size:1rem;color:#aaa">Live log</h2>
<div class="log-box" id="log"></div>
<p class="footer">auto-refreshes every 10 s · <a href="/api/status" style="color:#7c6df0">JSON API</a></p>

<script>
async function load(){
  const res=await fetch('/api/status');
  const rows=await res.json();
  const el=document.getElementById('repos');
  el.innerHTML=rows.map(r=>{
    const badges=[
      r.conflict?'<span class="badge conflict">conflict</span>':'<span class="badge ok">✓ ok</span>',
      r.dirty?'<span class="badge dirty">dirty</span>':'',
    ].join('');
    const ahead=r.ahead>0?'↑'+r.ahead+' ':'';
    const behind=r.behind>0?'↓'+r.behind+' ':'';
    return '<div class="card">'
      +'<div class="card-name">'+r.name+'</div>'
      +'<div class="card-meta">'+r.branch+' · '+ahead+behind+(r.last_sync||'never synced')+'</div>'
      +badges
      +(r.error?'<div style="color:#ff6b6b;font-size:.75rem;margin-top:6px">'+r.error+'</div>':'')
      +'<div class="btns">'
      +'<button class="btn-pull" onclick="op(\'pull\',\''+r.name+'\')">Pull</button>'
      +'<button class="btn-push" onclick="op(\'push\',\''+r.name+'\')">Push</button>'
      +'</div></div>';
  }).join('');
}
async function op(action,name){
  await fetch('/api/'+action+'?repo='+encodeURIComponent(name),{method:'POST'});
  load();
}
load();
setInterval(load,10000);

// Live log via SSE
const logEl=document.getElementById('log');
const es=new EventSource('/api/log');
es.onmessage=e=>{
  const p=document.createElement('p');
  const t=e.data;
  p.className=t.includes('[ERROR]')?'log-err':t.includes('[WARN]')?'log-warn':t.includes('[DEBUG]')?'log-dbg':'log-info';
  p.textContent=t;
  logEl.appendChild(p);
  logEl.scrollTop=logEl.scrollHeight;
  if(logEl.children.length>300) logEl.removeChild(logEl.firstChild);
};
</script>
</body>
</html>`
}
