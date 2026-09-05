window.onerror = (msg, src, line, col) => {
  const d = document.createElement("pre");
  d.style.cssText = "position:fixed;inset:60px 20px auto 20px;z-index:999;padding:14px;" +
    "background:#2a1015;color:#ff8080;border:1px solid #ff5f5f;border-radius:8px;" +
    "font:12px ui-monospace,Consolas,monospace;white-space:pre-wrap;";
  d.textContent = `JS ERROR\n${msg}\nat ${src}:${line}:${col}`;
  document.body.appendChild(d);
};

const go = () => window.go.main.App;
const rt = () => window.runtime;
const $  = (id) => document.getElementById(id);

let state    = { repos: [], loggedIn: false, watching: false };
let selected = null;
let detail   = null;

const esc = (s) => String(s ?? "").replace(/[&<>"]/g, c =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

function toast(msg, kind = "") {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast " + kind;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => t.classList.add("hidden"), 4200);
}

function logLine(text) {
  if (!text) return;
  const box = $("log");
  const p = document.createElement("p");
  p.textContent = text;
  if (/\[ERROR\]|FAILED|CONFLICT/.test(text))      p.className = "l-err";
  else if (/\[WARN\]/.test(text))                  p.className = "l-warn";
  else if (/pushed|pulled|committed/.test(text))   p.className = "l-ok";
  box.appendChild(p);
  box.scrollTop = box.scrollHeight;
  while (box.children.length > 500) box.removeChild(box.firstChild);
  $("sb-last").textContent = text.replace(/^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d /, "");
}

// ============================================================ render

function render() {
  $("ver").textContent = state.version || "";
  $("view-signin").classList.toggle("hidden",  state.loggedIn);
  $("view-main").classList.toggle("hidden",   !state.loggedIn);

  $("watch-label").textContent = state.watching ? "Watching" : "Idle";
  $("watch-dot").classList.toggle("on", state.watching);
  $("btn-syncall").disabled = !state.repos.length;

  renderSidebar();

  if (selected && !state.repos.some(r => r.name === selected)) selected = null;
  $("blank").classList.toggle("hidden",  !!selected);
  $("detail").classList.toggle("hidden", !selected);
}

function statusClass(r) {
  if (r.error || r.conflict) return "bad";
  if (!r.enabled)            return "idle";
  if (r.behind)              return "behind";
  if (r.ahead)               return "ahead";
  if (r.dirty)               return "dirty";
  return "";
}

function renderSidebar() {
  $("side-list").innerHTML = state.repos.map(r => {
    const badge = r.behind ? `↓${r.behind}` : r.ahead ? `↑${r.ahead}` : r.dirty ? "•" : "";
    return `
    <div class="item ${r.name === selected ? "sel" : ""} ${r.enabled ? "" : "off"}"
         onclick="select('${esc(r.name)}')">
      <span class="item-dot ${statusClass(r)}"></span>
      <div class="item-body">
        <div class="item-name">${r.busy ? '<span class="spin">⟳</span> ' : ""}${esc(r.name)}</div>
        <div class="item-sub">${esc(r.lastSync || "never synced")}</div>
      </div>
      <span class="item-badge">${badge}</span>
    </div>`;
  }).join("");
}

function renderDetail() {
  if (!detail) return;
  const d = detail;

  $("d-name").textContent  = d.name;
  $("d-track").textContent = d.tracking || d.branch || "";
  $("d-path").textContent  = d.path;
  $("d-path").title        = d.path;

  const alert = $("d-alert");
  if (d.error || d.conflict) {
    alert.textContent = d.error || `Conflict: ${d.conflict}`;
    alert.className = "alert";
  } else if (!d.enabled) {
    alert.textContent = "Automatic syncing is paused for this repository.";
    alert.className = "alert warn";
  } else {
    alert.className = "alert hidden";
  }

  setStat("s-in",    d.behind,  "cold");
  setStat("s-out",   d.ahead,   "hot");
  setStat("s-dirty", d.changed, "dirty");
  $("s-sync").textContent = d.lastSync || "never";

  // pending
  const groups = [];
  if (d.incoming?.length) groups.push(group("in",  "Incoming",  d.incoming));
  if (d.outgoing?.length) groups.push(group("out", "Outgoing",  d.outgoing));
  $("pending-body").innerHTML = groups.length
    ? groups.join("")
    : `<div class="empty-line">Nothing waiting — this repository is in step with ${esc(d.tracking || "its remote")}.</div>`;

  // files
  $("files-count").textContent = d.files?.length ? `(${d.files.length})` : "";
  $("files-body").innerHTML = d.files?.length
    ? d.files.map(f => `
      <div><div class="frow">
        <span class="ftag ${esc(f.status)}">${esc(f.status)}</span>
        <span class="fpath" title="${esc(f.path)}">${esc(f.path)}</span>
      </div></div>`).join("")
    : `<div class="empty-line">Working tree is clean.</div>`;

  // commits
  $("commits-body").innerHTML = d.commits?.length
    ? d.commits.map(c => `
      <div><div class="crow">
        <span class="chash">${esc(c.hash)}</span>
        <span class="csub" title="${esc(c.subject)}">${esc(c.subject)}</span>
        <span class="cwhen">${esc(c.when)}</span>
      </div></div>`).join("")
    : `<div class="empty-line">No commits yet.</div>`;
}

function group(kind, label, lines) {
  const arrow = kind === "in" ? "↓" : "↑";
  return `<div class="pend-group">
    <div class="pend-head ${kind}">${arrow} ${label} · ${lines.length}</div>
    ${lines.map(l => `<div class="pend-line">${esc(l)}</div>`).join("")}
  </div>`;
}

function setStat(id, n, cls) {
  const el = $(id);
  el.querySelector(".stat-n").textContent = n || 0;
  el.className = "stat " + (n ? cls : "zero");
}

// ============================================================ data

async function refresh(fetch = false) {
  try {
    state = await go().GetState(fetch);
    if (!selected && state.repos.length) selected = state.repos[0].name;
    render();
    if (selected) await loadDetail();
  } catch (e) { toast(String(e), "err"); }
}

async function loadDetail() {
  try {
    detail = await go().GetDetail(selected);
    renderDetail();
  } catch (e) { toast(String(e), "err"); }
}

async function select(name) {
  selected = name;
  render();
  await loadDetail();
}

// ============================================================ actions

async function op(kind) {
  if (!selected) return;
  const btn = $("d-" + kind.toLowerCase());
  btn.disabled = true;
  try {
    const res = await go()[kind](selected);
    if (res.error) toast(res.error, "err");
    else           toast(res.summary, "ok");
    (res.lines || []).forEach(l => logLine("    " + l));
  } catch (e) { toast(String(e), "err"); }
  btn.disabled = false;
  await refresh();
}

async function syncAll() {
  const b = $("btn-syncall");
  b.disabled = true; b.textContent = "Syncing…";
  try { await go().SyncAll(); toast("All repositories synced", "ok"); }
  catch (e) { toast(String(e), "err"); }
  b.textContent = "Sync all";
  await refresh();
}

async function toggleWatch() {
  try { state.watching = await go().ToggleWatch(); render(); }
  catch (e) { toast(String(e), "err"); }
}

// ============================================================ settings modal

function openCfg() {
  if (!detail) return;
  $("cfg-title").textContent = detail.name;
  $("cfg-interval").value = detail.interval;
  $("cfg-enabled").checked = detail.enabled;
  $("modal-cfg").classList.remove("hidden");
}
const closeCfg = () => $("modal-cfg").classList.add("hidden");

async function saveCfg() {
  try {
    const mins = parseInt($("cfg-interval").value, 10);
    if (mins !== detail.interval) await go().SetInterval(selected, mins);
    const on = $("cfg-enabled").checked;
    if (on !== detail.enabled) await go().SetEnabled(selected, on);
    closeCfg();
    toast("Settings saved", "ok");
    await refresh();
  } catch (e) { toast(String(e), "err"); }
}

async function removeRepo() {
  if (!confirm(`Stop syncing ${selected}?\n\nThe folder and its files are left untouched.`)) return;
  try {
    await go().RemoveRepo(selected);
    closeCfg();
    selected = null;
    toast("Repository removed", "ok");
    await refresh();
  } catch (e) { toast(String(e), "err"); }
}

// ============================================================ diff

async function showDiff() {
  if (!selected) return;
  $("diff-title").textContent = `Changes — ${selected}`;
  $("diff-body").textContent = "loading…";
  $("modal-diff").classList.remove("hidden");
  try {
    const out = await go().Diff(selected);
    $("diff-body").innerHTML = out.trim()
      ? out.split("\n").map(l => {
          const c = l.startsWith("+++") || l.startsWith("---") || l.startsWith("diff") || l.startsWith("@@")
            ? "hdr" : l.startsWith("+") ? "add" : l.startsWith("-") ? "del" : "";
          return `<span class="${c}">${esc(l)}</span>`;
        }).join("\n")
      : "No changes.";
  } catch (e) { $("diff-body").textContent = String(e); }
}
const closeDiff = () => $("modal-diff").classList.add("hidden");

// ============================================================ add repo

function openAdd() {
  $("add-err").textContent = "";
  $("modal-add").classList.remove("hidden");
  loadGitHubList();
}
const closeAdd = () => $("modal-add").classList.add("hidden");

async function loadGitHubList() {
  const sel = $("gh-list");
  sel.innerHTML = "<option>loading…</option>";
  try {
    const all = await go().ListGitHubRepos();
    const taken = new Set(state.repos.map(r => r.slug).filter(Boolean));
    const free = all.filter(s => !taken.has(s));
    sel.innerHTML = free.length
      ? free.map(s => `<option>${esc(s)}</option>`).join("")
      : "<option disabled>every repository is already added</option>";
  } catch (e) {
    sel.innerHTML = `<option disabled>${esc(String(e))}</option>`;
  }
}

async function confirmAdd() {
  const clone = !$("pane-clone").classList.contains("hidden");
  const err = $("add-err");
  const btn = $("btn-add-confirm");
  err.textContent = "";
  btn.disabled = true; btn.textContent = "Working…";
  try {
    if (clone) {
      const slug = $("gh-list").value;
      if (!slug) throw "Choose a repository first.";
      await go().CloneAndAdd(slug, $("clone-dest").value.trim());
      selected = slug.split("/")[1];
    } else {
      const p = $("local-path").value.trim();
      if (!p) throw "Choose a folder first.";
      await go().AddLocal(p);
      selected = p.replace(/[\\/]$/, "").split(/[\\/]/).pop();
    }
    closeAdd();
    toast("Repository added", "ok");
    await refresh(true);
  } catch (e) { err.textContent = String(e); }
  btn.disabled = false; btn.textContent = "Add repository";
}

// ============================================================ auth

async function signIn() {
  const err = $("signin-err");
  const btn = $("btn-signin");
  err.textContent = "";
  btn.disabled = true; btn.textContent = "Connecting…";
  try {
    const who = await go().SignIn($("pat").value);
    $("pat").value = "";
    toast(`Signed in as ${who}`, "ok");
    await refresh(true);
  } catch (e) { err.textContent = String(e); }
  btn.disabled = false; btn.textContent = "Connect account";
}

// ============================================================ wiring

function init() {
  $("btn-watch").onclick   = toggleWatch;
  $("btn-syncall").onclick = syncAll;
  $("btn-add").onclick     = openAdd;
  $("btn-add-2").onclick   = openAdd;
  $("btn-signin").onclick  = signIn;
  $("btn-add-confirm").onclick = confirmAdd;

  $("d-pull").onclick = () => op("Pull");
  $("d-push").onclick = () => op("Push");
  $("d-sync").onclick = () => op("Sync");
  $("d-menu").onclick = openCfg;

  $("cfg-save").onclick   = saveCfg;
  $("cfg-remove").onclick = removeRepo;

  $("pat").addEventListener("keydown", e => { if (e.key === "Enter") signIn(); });

  $("lnk-token").onclick = e => {
    e.preventDefault();
    rt().BrowserOpenURL("https://github.com/settings/tokens/new?scopes=repo&description=synchronize");
  };
  $("d-path").onclick = () => detail && go().OpenFolder(detail.path);

  $("btn-browse").onclick = async () => {
    const p = await go().PickFolder();
    if (p) $("local-path").value = p;
  };
  $("btn-browse-dest").onclick = async () => {
    const p = await go().PickFolder();
    if (p) $("clone-dest").value = p;
  };

  document.querySelectorAll(".tab").forEach(t => {
    t.onclick = () => {
      document.querySelectorAll(".tab").forEach(x => x.classList.remove("active"));
      t.classList.add("active");
      const clone = t.dataset.tab === "clone";
      $("pane-clone").classList.toggle("hidden", !clone);
      $("pane-local").classList.toggle("hidden",  clone);
    };
  });

  $("sb-toggle").onclick = () => {
    const open = $("logpane").classList.toggle("hidden");
    $("sb-caret").classList.toggle("open", !open);
  };

  document.querySelectorAll(".modal").forEach(m => {
    m.onclick = e => { if (e.target === m) m.classList.add("hidden"); };
  });
  document.addEventListener("keydown", e => {
    if (e.key === "Escape")
      document.querySelectorAll(".modal").forEach(m => m.classList.add("hidden"));
  });

  rt().EventsOn("log", logLine);
  rt().EventsOn("changed", () => refresh());
  rt().EventsOn("error", m => toast(m, "err"));

  go().RecentLog(60).then(ls => ls.forEach(logLine)).catch(() => {});
  refresh(true);
  setInterval(() => refresh(false), 20000);
}

// The Wails runtime is injected before our bundle runs, and this script sits at
// the end of <body>, so the DOM is ready either way.
if (document.readyState === "loading")
  document.addEventListener("DOMContentLoaded", init);
else
  init();
