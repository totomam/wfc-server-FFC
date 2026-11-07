package api

import (
	"net/http"
	"strconv"
	"strings"
)

// FFC web presence widget (ffc-decomp ROADMAP #39).
//   GET /ffc/widget[?top=N]   small static HTML page (no external JS/CSS/fonts) that reads /api/ffc/online and
//   /api/ffc/leaderboard from the same server and refreshes every 60 s. Embeddable:
//   <iframe src="https://SERVER/ffc/widget?top=5" width="320" height="360" style="border:0"></iframe>
// Player names are inserted with textContent only (never innerHTML). The CSP allows only same-origin fetches and the
// page's own inline script/style; frame-ancestors * so any site (Discord-linked page, forum, wiki) may embed it.

const ffcWidgetHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>FFC Online</title>
<style>
:root{--bg:#fff;--fg:#1b1b1b;--mut:#666;--line:#ddd;--acc:#2a7a3b}
@media (prefers-color-scheme:dark){:root{--bg:#16181c;--fg:#eee;--mut:#9a9a9a;--line:#333;--acc:#5fc46f}}
body{margin:0;padding:12px;background:var(--bg);color:var(--fg);font:14px/1.35 system-ui,sans-serif}
h1{font-size:15px;margin:0 0 8px}
.n{display:flex;gap:10px;margin-bottom:10px}
.n div{flex:1;border:1px solid var(--line);border-radius:6px;padding:6px;text-align:center}
.n b{display:block;font-size:20px;color:var(--acc)}
.n span{font-size:11px;color:var(--mut)}
table{width:100%;border-collapse:collapse}
td,th{padding:3px 4px;border-bottom:1px solid var(--line);text-align:left}
th{font-size:11px;color:var(--mut);font-weight:normal}
td.r,th.r{text-align:right}
#st{font-size:11px;color:var(--mut);margin-top:6px}
</style></head><body>
<h1>Fossil Fighters: Champions online</h1>
<div class="n"><div><b id="on">-</b><span>online</span></div><div><b id="mm">-</b><span>searching / in match</span></div><div><b id="bt">-</b><span>in battle</span></div></div>
<table><thead><tr><th>#</th><th>Player</th><th class="r">Elo</th><th class="r">W-L-D</th></tr></thead><tbody id="lb"></tbody></table>
<div id="st">loading...</div>
<script>
(function(){
var TOP=__TOP__;
function el(t,x,c){var e=document.createElement(t);e.textContent=x;if(c)e.className=c;return e}
function get(u){return fetch(u,{cache:"no-store"}).then(function(r){if(!r.ok)throw new Error(r.status);return r.json()})}
function load(){
 get("/api/ffc/online").then(function(o){
  document.getElementById("on").textContent=o.online;
  document.getElementById("mm").textContent=o.matchmaking;
  document.getElementById("bt").textContent=o.in_battle;
 }).catch(function(){});
 get("/api/ffc/leaderboard").then(function(d){
  var tb=document.getElementById("lb");tb.textContent="";
  var ps=(d.players||[]).slice(0,TOP);
  ps.forEach(function(p){var tr=document.createElement("tr");
   tr.appendChild(el("td",String(p.rank)));
   tr.appendChild(el("td",p.name||("#"+p.pid)));
   tr.appendChild(el("td",String(Math.round(p.elo)),"r"));
   tr.appendChild(el("td",p.wins+"-"+p.losses+"-"+p.draws,"r"));
   tb.appendChild(tr)});
  if(!ps.length){var tr=document.createElement("tr"),td=el("td","no ranked matches yet");td.colSpan=4;tr.appendChild(td);tb.appendChild(tr)}
  document.getElementById("st").textContent=(d.season?"season "+d.season+" | ":"")+d.matches+" matches | updated "+new Date().toLocaleTimeString();
 }).catch(function(e){document.getElementById("st").textContent="server unreachable ("+e.message+")"});
}
load();setInterval(load,60000);
})();
</script></body></html>
`

const ffcWidgetCSP = "default-src 'none'; connect-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-ancestors *"

func ffcWidgetPage(top int) []byte {
	if top < 1 || top > 50 {
		top = 10
	}
	return []byte(strings.Replace(ffcWidgetHTML, "__TOP__", strconv.Itoa(top), 1))
}

func HandleFFCWidget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	top, _ := strconv.Atoi(r.URL.Query().Get("top"))
	body := ffcWidgetPage(top)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", ffcWidgetCSP)
	h.Set("Cache-Control", "public, max-age=300")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
