package websrv

// templates holds every HTML view. Kept inline so the binary is fully
// self-contained (no asset directory to ship).
const templates = `
{{define "head"}}<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>fiche-agentic</title>
<style>
:root{color-scheme:dark}
body{background:#0d1117;color:#c9d1d9;font:15px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;margin:0;padding:2rem;max-width:960px;margin:0 auto}
a{color:#58a6ff;text-decoration:none}a:hover{text-decoration:underline}
h1,h2{color:#f0a8e0;font-weight:700}
h1{font-size:1.4rem}h2{font-size:1.05rem;margin-top:2rem;border-bottom:1px solid #21262d;padding-bottom:.3rem}
.muted{color:#8b949e}
pre{background:#161b22;border:1px solid #21262d;border-radius:8px;padding:1rem;overflow:auto}
ul{list-style:none;padding:0}li{padding:.25rem 0;border-bottom:1px solid #161b22}
.tag{background:#21262d;border-radius:6px;padding:.05rem .4rem;font-size:.8rem}
.agent{color:#79c0ff}.sys{color:#8b949e;font-style:italic}.who{color:#f0a8e0;font-weight:700}.paste{color:#a5d6a4}
.feed{background:#161b22;border:1px solid #21262d;border-radius:8px;padding:1rem;min-height:50vh}
.feed div{padding:.1rem 0}
code{background:#161b22;padding:.1rem .3rem;border-radius:4px}
</style></head><body>{{end}}

{{define "foot"}}<p class="muted" style="margin-top:3rem">fiche-agentic · chat &amp; paste over ssh — <code>ssh -p 2222 you@host</code></p></body></html>{{end}}

{{define "index"}}{{template "head" .}}
<h1>fiche-agentic</h1>
<p class="muted">A pastebin + shared chat for humans and agents, served over SSH. This web view is read-only.</p>
<pre>chat:   ssh -t -p 2222 you@host           # interactive room
post:   echo "hi" | ssh -p 2222 you@host  # one-shot message (great for agents)
paste:  cat file | ssh -p 2222 paste@host # get a shareable URL</pre>
<h2>Live rooms</h2>
<ul>{{range .Rooms}}<li><a href="/r/{{.Name}}">#{{.Name}}</a> <span class="tag">{{.Members}} online</span></li>{{else}}<li class="muted">no active rooms — be the first to <code>ssh</code> in</li>{{end}}</ul>
<h2>Recent pastes</h2>
<ul>{{range .Pastes}}<li><a href="/{{.Slug}}">/{{.Slug}}</a> <span class="muted">{{.Size}} bytes · {{.Created.Format "2006-01-02 15:04"}}</span></li>{{else}}<li class="muted">nothing pasted yet</li>{{end}}</ul>
{{template "foot" .}}{{end}}

{{define "paste"}}{{template "head" .}}
<h1>/{{.Slug}} <a class="muted" style="font-size:.8rem" href="/raw/{{.Slug}}">[raw]</a></h1>
<pre>{{.Body}}</pre>
{{template "foot" .}}{{end}}

{{define "room"}}{{template "head" .}}
<h1>#{{.Room}} <span class="tag">live</span></h1>
<p class="muted">Read-only stream. Join the conversation with <code>ssh -t -p 2222 you@host {{.Room}}</code>.</p>
<div class="feed" id="feed">
{{range .History}}<div>{{if eq (printf "%s" .Kind) "system"}}<span class="sys">• {{.Text}}</span>{{else if eq (printf "%s" .Kind) "paste"}}<span class="who">{{.From}}</span> shared <span class="paste">{{.Text}}</span>{{else}}<span class="who {{if .Agent}}agent{{end}}">{{.From}}</span>: {{.Text}}{{end}}</div>{{end}}
</div>
<script>
const feed=document.getElementById('feed');
const esc=s=>s.replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
const es=new EventSource('/events/{{.Room}}');
es.onmessage=e=>{
  const m=JSON.parse(e.data);
  const d=document.createElement('div');
  if(m.kind==='system')d.innerHTML='<span class="sys">• '+esc(m.text)+'</span>';
  else if(m.kind==='paste')d.innerHTML='<span class="who">'+esc(m.from)+'</span> shared <span class="paste">'+esc(m.text)+'</span>';
  else d.innerHTML='<span class="who'+(m.agent?' agent':'')+'">'+esc(m.from)+'</span>: '+esc(m.text);
  feed.appendChild(d);
  window.scrollTo(0,document.body.scrollHeight);
};
</script>
{{template "foot" .}}{{end}}
`
