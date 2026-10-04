// Command deployd 部署操作面板：在宿主机上提供可视化界面，
// 手动执行「拉取代码 / 打包后端 / 打包前端 / 滚动更新」并实时回显日志。
//
// 运行在宿主机（不在 K8s 内），依赖宿主机 docker / kubectl / git / k3s ctr。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"
)

const defaultRepo = "/root/ykwork/campus-service-platform"

var (
	repo    = envOr("DEPLOY_REPO", defaultRepo)
	port    = envOr("DEPLOY_PORT", "8091")
	token   = os.Getenv("DEPLOY_TOKEN") // 为空则仅依赖 127.0.0.1 绑定
	kubeCfg = "/etc/rancher/k3s/k3s.yaml"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// ---------- Job 管理 ----------

type Job struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Status string    `json:"status"` // running | success | failed
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	mu     sync.Mutex
	Lines  []string `json:"lines"`
}

func (j *Job) append(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Lines = append(j.Lines, line)
}

func (j *Job) snapshot() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]string, len(j.Lines))
	copy(out, j.Lines)
	return out
}

type jobStore struct {
	mu   sync.Mutex
	jobs map[string]*Job
	seq  int
}

func newJobStore() *jobStore { return &jobStore{jobs: map[string]*Job{}} }

func (s *jobStore) add(name string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	j := &Job{
		ID:     fmt.Sprintf("%d", s.seq),
		Name:   name,
		Status: "running",
		Start:  time.Now(),
		Lines:  []string{},
	}
	s.jobs[j.ID] = j
	return j
}

func (s *jobStore) list() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID > out[k].ID })
	return out
}

func (s *jobStore) get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

var jobs = newJobStore()

// ---------- 命令定义 ----------

func bashScript(action string) (string, error) {
	reg := "ghcr.io/kkky1/campus-service-platform"
	common := "export KUBECONFIG=" + kubeCfg + "\ncd " + repo + "\n"
	tag := func(svc string) string {
		return fmt.Sprintf("TAG=\"manual-$(date +%%Y%%m%%d%%H%%M%%S)\"\necho \"$TAG\" > bin/last-%s-tag\n", svc)
	}
	img := func(svc string) string { return reg + "/" + svc }

	switch action {
	case "git-pull":
		return common + "git pull --ff-only origin main 2>&1 || echo '⚠️ git pull 失败（可忽略，继续用本地代码）'", nil

	case "build-backend":
		return common + "set -e\n" + tag("backend") +
			"./scripts/build.sh bin/campus-server\n" +
			"docker build -f Dockerfile.prebuilt -t " + img("backend") + ":$TAG .\n" +
			"docker save " + img("backend") + ":$TAG | k3s ctr images import -\n" +
			"echo '✅ 后端镜像打包完成: backend:'$TAG", nil

	case "build-frontend":
		return common + "set -e\n" + tag("frontend") +
			"docker build -f Dockerfile.front -t " + img("frontend") + ":$TAG .\n" +
			"docker save " + img("frontend") + ":$TAG | k3s ctr images import -\n" +
			"echo '✅ 前端镜像打包完成: frontend:'$TAG", nil

	case "rollout-backend":
		return common + "set -e\nTAG=$(cat bin/last-backend-tag 2>/dev/null || echo latest)\n" +
			"echo '滚动更新后端 → backend:'$TAG\n" +
			"kubectl -n campus set image deployment/backend backend=" + img("backend") + ":$TAG\n" +
			"kubectl -n campus rollout status deployment/backend --timeout=180s\n" +
			"echo '✅ 后端滚动更新完成: backend:'$TAG", nil

	case "rollout-frontend":
		return common + "set -e\nTAG=$(cat bin/last-frontend-tag 2>/dev/null || echo latest)\n" +
			"echo '滚动更新前端 → frontend:'$TAG\n" +
			"kubectl -n campus set image deployment/frontend nginx=" + img("frontend") + ":$TAG\n" +
			"kubectl -n campus rollout status deployment/frontend --timeout=180s\n" +
			"echo '✅ 前端滚动更新完成: frontend:'$TAG", nil

	case "build-rollout-backend":
		return mustScript("build-backend") + "\n" + mustScript("rollout-backend"), nil
	case "build-rollout-frontend":
		return mustScript("build-frontend") + "\n" + mustScript("rollout-frontend"), nil

	case "deploy-all":
		return common + "set -e\n" +
			"git pull --ff-only origin main 2>&1 || echo '⚠️ git pull 失败（继续）'\n" +
			mustScript("build-rollout-backend") + "\n" +
			mustScript("build-rollout-frontend") + "\n" +
			"echo '✅ 全部服务打包 + 滚动更新完成'", nil

	default:
		return "", fmt.Errorf("未知操作: %s", action)
	}
}

// mustScript 对已知合法的 action 取脚本（出错时回退为错误输出脚本）。
func mustScript(action string) string {
	s, err := bashScript(action)
	if err != nil {
		return "echo '❌ " + err.Error() + "'"
	}
	return s
}

// ---------- 执行 ----------

func runJob(j *Job, action string) {
	script, err := bashScript(action)
	if err != nil {
		j.append("❌ " + err.Error())
		j.Status = "failed"
		j.End = time.Now()
		return
	}
	cmd := exec.Command("bash", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		j.append("❌ stdout: " + err.Error())
		j.Status = "failed"
		j.End = time.Now()
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		j.append("❌ stderr: " + err.Error())
		j.Status = "failed"
		j.End = time.Now()
		return
	}
	if err := cmd.Start(); err != nil {
		j.append("❌ 启动失败: " + err.Error())
		j.Status = "failed"
		j.End = time.Now()
		return
	}
	go scanLines(stdout, j)
	go scanLines(stderr, j)
	if err := cmd.Wait(); err != nil {
		j.append("❌ 命令退出码非 0: " + err.Error())
		j.Status = "failed"
	} else {
		j.Status = "success"
	}
	j.End = time.Now()
}

func scanLines(r io.Reader, j *Job) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		j.append(sc.Text())
	}
}

// ---------- HTTP ----------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func authOK(r *http.Request) bool {
	if token == "" {
		return true
	}
	return r.Header.Get("X-Deploy-Token") == token || r.URL.Query().Get("token") == token
}

type api struct{}

func (api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 首页（静态 HTML）无需鉴权，否则用户无法打开页面输入令牌；
	// 仅 /api/* 接口要求令牌。
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, htmlPage)
		return
	}
	if !authOK(r) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, map[string]string{"error": "unauthorized"})
		return
	}
	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, htmlPage)
	case "/api/status":
		writeJSON(w, collectStatus())
	case "/api/jobs":
		writeJSON(w, jobs.list())
	case "/api/run":
		handleRun(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	if _, err := bashScript(body.Action); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	j := jobs.add(body.Action)
	go runJob(j, body.Action)
	writeJSON(w, j)
}

// collectStatus 汇总应用状态 + 工作负载。
func collectStatus() map[string]interface{} {
	out := map[string]interface{}{}
	if b, err := exec.Command("curl", "-s", "http://127.0.0.1:8080/api/deploy/status").Output(); err == nil {
		var app interface{}
		if json.Unmarshal(b, &app) == nil {
			out["app"] = app
		}
	}
	out["workloads"] = shellText("kubectl -n campus get deploy,sts -o wide 2>&1")
	out["pods"] = shellText("kubectl -n campus get pods -o wide 2>&1")
	return out
}

func shellText(cmdline string) string {
	b, err := exec.Command("bash", "-c", "export KUBECONFIG="+kubeCfg+"; "+cmdline).CombinedOutput()
	if err != nil {
		return string(b) + "\n(err: " + err.Error() + ")"
	}
	return string(b)
}

func main() {
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: api{}}
	log.Printf("部署面板已启动: http://127.0.0.1:%s  (repo=%s, token=%s)", port, repo, map[bool]string{true: "已启用", false: "未启用"}[token != ""])
	log.Fatal(srv.ListenAndServe())
}

// ---------- 前端 ----------

const htmlPage = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>部署操作面板 · 校园服务平台</title>
<style>
:root{--bg:#0f172a;--card:#1e293b;--border:#334155;--ink:#e2e8f0;--muted:#94a3b8;--ok:#22c55e;--bad:#ef4444;--run:#f59e0b;--btn:#2563eb;--btn2:#0ea5e9}
*{box-sizing:border-box}
body{margin:0;font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;background:var(--bg);color:var(--ink);padding:20px}
.wrap{max-width:1080px;margin:0 auto}
h1{font-size:20px;margin:0 0 4px}
.sub{color:var(--muted);font-size:13px;margin-bottom:18px}
.bar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:18px}
.btn{border:0;border-radius:10px;padding:9px 14px;font-size:13px;cursor:pointer;color:#fff;background:var(--btn);font-weight:600}
.btn:hover{filter:brightness(1.1)}
.btn.b2{background:var(--btn2)}
.btn.b3{background:#7c3aed}
.btn.ghost{background:#334155}
.btn:disabled{opacity:.5;cursor:not-allowed}
.badge{display:inline-flex;align-items:center;gap:6px;padding:6px 12px;border-radius:999px;background:var(--card);border:1px solid var(--border);font-size:13px}
.dot{width:9px;height:9px;border-radius:50%;background:var(--muted)}
.dot.ok{background:var(--ok)} .dot.bad{background:var(--bad)} .dot.run{background:var(--run);animation:pl 1.5s infinite}
@keyframes pl{50%{opacity:.35}}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(340px,1fr));gap:14px;margin-bottom:18px}
.card{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:16px}
.card h3{margin:0 0 8px;font-size:15px}
.card .kv{font-size:12px;color:var(--muted);margin:3px 0;word-break:break-all}
.card .btns{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
.panel{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:16px;margin-bottom:18px}
.panel h3{margin:0 0 10px;font-size:15px}
pre{background:#0b1220;border:1px solid var(--border);border-radius:10px;padding:12px;font-size:12px;line-height:1.5;overflow:auto;max-height:260px;white-space:pre-wrap;word-break:break-all;color:#cbd5e1}
.log{max-height:420px}
.token-row{display:flex;gap:8px;margin-bottom:16px}
input{flex:1;background:#0b1220;border:1px solid var(--border);border-radius:10px;padding:9px 12px;color:var(--ink);font-size:13px}
</style>
</head>
<body>
<div class="wrap">
  <h1>🚀 部署操作面板</h1>
  <div class="sub">手动打包（backend/frontend 镜像）+ 滚动更新，实时回显日志 · 仅本机访问</div>

  <div class="token-row">
    <input id="token" type="password" placeholder="访问令牌（若服务端未设置 DEPLOY_TOKEN 可留空）">
    <button class="btn ghost" onclick="saveToken()">保存</button>
  </div>

  <div class="bar">
    <span class="badge"><span class="dot" id="app-dot"></span><span id="app-text">加载中…</span></span>
    <button class="btn" onclick="run('git-pull')">拉取代码</button>
    <button class="btn b3" onclick="run('deploy-all')">一键部署全部</button>
  </div>

  <div class="grid">
    <div class="card">
      <h3>🔧 后端 backend</h3>
      <div class="kv" id="backend-info">加载中…</div>
      <div class="btns">
        <button class="btn" onclick="run('build-backend')">打包</button>
        <button class="btn b2" onclick="run('rollout-backend')">滚动更新</button>
        <button class="btn b3" onclick="run('build-rollout-backend')">打包+更新</button>
      </div>
    </div>
    <div class="card">
      <h3>🌐 前端 frontend</h3>
      <div class="kv" id="frontend-info">加载中…</div>
      <div class="btns">
        <button class="btn" onclick="run('build-frontend')">打包</button>
        <button class="btn b2" onclick="run('rollout-frontend')">滚动更新</button>
        <button class="btn b3" onclick="run('build-rollout-frontend')">打包+更新</button>
      </div>
    </div>
  </div>

  <div class="panel">
    <h3>工作负载状态</h3>
    <pre id="workloads">加载中…</pre>
  </div>

  <div class="panel">
    <h3>执行日志</h3>
    <pre id="log" class="log">点击上方按钮开始…</pre>
  </div>
</div>

<script>
var curJob = null, timer = null;
function token(){ return localStorage.getItem('deploy_token') || '' }
function saveToken(){ localStorage.setItem('deploy_token', document.getElementById('token').value); refresh(); }
document.getElementById('token').value = token();
function hdrs(){ var t = token(); return t ? {'X-Deploy-Token': t} : {} }

async function run(action){
  var r = await fetch('/api/run', {method:'POST', headers:Object.assign({'Content-Type':'application/json'}, hdrs()), body:JSON.stringify({action:action})});
  var j = await r.json();
  if (r.status === 401){ alert('令牌错误'); return; }
  if (j.error){ alert(j.error); return; }
  curJob = j.id;
  document.getElementById('log').textContent = '';
  poll();
}
async function poll(){
  if (!curJob) return;
  var r = await fetch('/api/jobs', {headers:hdrs()});
  var arr = await r.json();
  var j = arr.find(function(x){ return x.id === curJob });
  if (!j) return;
  document.getElementById('log').textContent = j.lines.join('\n');
  var el = document.getElementById('log'); el.scrollTop = el.scrollHeight;
  if (j.status === 'running'){ timer = setTimeout(poll, 1000); }
  else { curJob = null; refresh(); }
}
async function refresh(){
  var r = await fetch('/api/status', {headers:hdrs()});
  if (r.status === 401){ document.getElementById('app-text').textContent = '令牌错误'; return; }
  var s = await r.json();
  document.getElementById('workloads').textContent = (s.workloads || '') + '\n' + (s.pods || '');
  var app = s.app || {};
  var dot = document.getElementById('app-dot'), txt = document.getElementById('app-text');
  dot.className = 'dot ' + (app.ready ? 'ok' : 'bad');
  txt.textContent = '版本 ' + (app.version || '?') + (app.ready ? ' · 运行正常' : ' · 降级');
  renderSvc('backend-info', 'backend', app);
  renderSvc('frontend-info', 'frontend', app);
}
function renderSvc(el, name, app){
  var e = document.getElementById(el);
  var deps = (app.deps || []).map(function(d){ return d.name + (d.healthy ? '✅' : '❌') }).join(' ');
  e.textContent = '就绪: ' + (app.ready ? '是' : '否') + ' · Pod: ' + (app.pod || '-') + ' · 依赖: ' + deps;
}
refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>`
