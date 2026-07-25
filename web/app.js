"use strict";
// VARA Hub — a minimal same-origin SPA over the RFC-0021 read API and the
// RFC-0019/0020 control planes. No framework: it is served statically by
// `vara serve --hub` and authenticates via the httpOnly session cookie.

const app = document.getElementById("app");
const topbar = document.getElementById("topbar");

// --- tiny helpers ------------------------------------------------------------

const esc = (s) =>
  String(s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
const short = (id) => (id ? id.slice(0, 10) : "");
const when = (ts) => (ts ? new Date(ts).toLocaleString() : "");

// encPath encodes a browse path for a URL, preserving "/" as segment separators
// (each segment is individually percent-encoded). "" → "" (the root).
const encPath = (p) =>
  String(p || "").split("/").filter(Boolean).map(encodeURIComponent).join("/");

// Structural icons are inline SVG (Lucide-style, inheriting currentColor) — never
// emoji, so they stay crisp, themeable, and consistent. aria-hidden: decorative.
const svgWrap = (size, inner) =>
  `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${inner}</svg>`;
const ICON = {
  logo: (s = 20) => svgWrap(s, '<circle cx="12" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><circle cx="18" cy="6" r="3"/><path d="M18 9v1a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V9"/><path d="M12 12v3"/>'),
  folder: (s = 16) => svgWrap(s, '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>'),
  file: (s = 16) => svgWrap(s, '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>'),
  repo: (s = 16) => svgWrap(s, '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>'),
  branch: (s = 16) => svgWrap(s, '<line x1="6" x2="6" y1="3" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>'),
  commit: (s = 16) => svgWrap(s, '<circle cx="12" cy="12" r="3"/><line x1="3" x2="9" y1="12" y2="12"/><line x1="15" x2="21" y1="12" y2="12"/>'),
  clock: (s = 16) => svgWrap(s, '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>'),
  binary: (s = 16) => svgWrap(s, '<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="M3.3 7 12 12l8.7-5"/><path d="M12 22V12"/>'),
};

// renderCode turns file text into a line-numbered, INERT code view: each line is
// escaped, so content is never treated as HTML — the client-side mirror of the
// server's B5 guarantee.
function renderCode(content) {
  const lines = String(content ?? "").split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop(); // trailing newline
  const rows = lines
    .map(
      (ln, i) =>
        `<div class="code-row"><span class="code-ln">${i + 1}</span><span class="code-lc">${esc(ln) || " "}</span></div>`
    )
    .join("");
  return `<div class="code">${rows}</div>`;
}

function toast(msg, isErr) {
  const t = document.getElementById("toast");
  t.textContent = msg;
  t.className = "toast" + (isErr ? " err" : "");
  t.hidden = false;
  setTimeout(() => (t.hidden = true), 3000);
}

// api issues a same-origin request (the session cookie rides automatically) and
// returns parsed JSON, throwing {status, code, message} on a non-2xx response.
async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(path, opts);
  if (resp.status === 204) return null;
  const text = await resp.text();
  const data = text ? JSON.parse(text) : null;
  if (!resp.ok) {
    const err = new Error((data && data.message) || resp.statusText);
    err.status = resp.status;
    err.code = data && data.code;
    throw err;
  }
  return data;
}

// guard runs a view loader, routing 401 to login and rendering 403 inline.
async function guard(loader) {
  try {
    await loader();
  } catch (e) {
    if (e.status === 401) {
      toast("Please log in", true);
      location.hash = "#/login";
    } else if (e.status === 403) {
      app.innerHTML = `<div class="card empty">You don't have permission to view this (${esc(e.code || "forbidden")}).</div>`;
    } else {
      app.innerHTML = `<div class="card empty">Error: ${esc(e.message)}</div>`;
    }
  }
}

// --- identity / chrome -------------------------------------------------------

let me = null;

async function refreshWhoami() {
  try {
    me = await api("GET", "/_vara/whoami");
  } catch {
    me = { id: "anonymous", anonymous: true };
  }
  const wa = document.getElementById("whoami");
  const nav = document.getElementById("nav");
  if (me && !me.anonymous) {
    topbar.hidden = false;
    nav.innerHTML = `<a href="#/">Repositories</a>`;
    const initial = esc((me.id || "?").slice(0, 1));
    wa.innerHTML = `<span class="avatar">${initial}</span><b>${esc(me.id)}</b> · <a href="#" id="logout">Sign out</a>`;
    document.getElementById("logout").onclick = async (ev) => {
      ev.preventDefault();
      try {
        await api("DELETE", "/_vara/sessions/current");
      } catch {}
      me = null;
      location.hash = "#/login";
      toast("Logged out");
    };
  } else {
    topbar.hidden = true;
  }
}

// --- views -------------------------------------------------------------------

function viewLogin() {
  topbar.hidden = true;
  app.innerHTML = `
    <div class="login-wrap">
      <div class="brand-lg">${ICON.logo(34)}<h1 style="margin:0">VARA Hub</h1></div>
      <p class="sub center">Sign in to continue</p>
      <div class="card">
        <label>Username</label><input id="u" autofocus />
        <label>Password</label><input id="p" type="password" />
        <div class="actions"><button class="primary" id="go" style="flex:1">Log in</button></div>
      </div>
    </div>`;
  const submit = async () => {
    const username = document.getElementById("u").value.trim();
    const password = document.getElementById("p").value;
    try {
      await api("POST", "/_vara/sessions?cookie=1", { username, password });
      await refreshWhoami();
      location.hash = "#/";
      toast("Welcome, " + username);
    } catch (e) {
      toast(e.status === 401 ? "Invalid credentials" : e.message, true);
    }
  };
  document.getElementById("go").onclick = submit;
  app.querySelectorAll("input").forEach((i) =>
    i.addEventListener("keydown", (e) => e.key === "Enter" && submit())
  );
}

async function viewDashboard() {
  const data = await api("GET", "/_vara/repositories");
  const repos = (data && data.repositories) || [];
  const rows = repos.length
    ? repos
        .map(
          (r) => `
      <div class="card row">
        <span class="rowicon dir">${ICON.repo(20)}</span>
        <div class="grow">
          <a class="repo-name" href="#/r/${encodeURIComponent(r.name)}">${esc(r.name)}</a>
          <div class="meta">${esc(r.visibility)} · owner ${esc(r.owner)}</div>
        </div>
        <span class="chip state">${esc(r.state)}</span>
      </div>`
        )
        .join("")
    : `<div class="card empty">No repositories yet.</div>`;
  app.innerHTML = `
    <h1>Repositories</h1>
    <p class="sub">Repositories you can access on this server.</p>
    ${rows}
    <h2>Create a repository</h2>
    <div class="card row">
      <div class="grow"><input id="newrepo" placeholder="repository name" /></div>
      <button class="primary" id="create">Create</button>
    </div>`;
  document.getElementById("create").onclick = async () => {
    const name = document.getElementById("newrepo").value.trim();
    if (!name) return;
    try {
      await api("POST", "/_vara/repositories", { name });
      toast("Created " + name);
      viewDashboard();
    } catch (e) {
      toast(e.message, true);
    }
  };
}

function repoTabs(name, active) {
  const tab = (id, label) =>
    `<a href="#/r/${encodeURIComponent(name)}${id ? "/" + id : ""}" class="${active === id ? "active" : ""}">${label}</a>`;
  return `<nav id="nav" style="margin-bottom:16px">
    ${tab("", "Overview")}${tab("tree", "Files")}${tab("history", "History")}${tab("branches", "Branches")}${tab("settings", "Settings")}
  </nav>`;
}

// crumbs renders a clickable breadcrumb for a browse path. The final segment of a
// blob path is plain text (you are already viewing it); every other segment links
// to its tree. Slashes in a path are encoded into one hash segment (encodeURI-
// Component), which the router decodes back.
function crumbs(name, path, isBlob) {
  const enc = encodeURIComponent(name);
  const out = [`<a href="#/r/${enc}/tree">${esc(name)}</a>`];
  const segs = String(path || "").split("/").filter(Boolean);
  let acc = "";
  segs.forEach((seg, i) => {
    acc = acc ? acc + "/" + seg : seg;
    const last = i === segs.length - 1;
    if (last && isBlob) {
      out.push(`<span>${esc(seg)}</span>`);
    } else {
      out.push(`<a href="#/r/${enc}/tree/${encodeURIComponent(acc)}">${esc(seg)}</a>`);
    }
  });
  return `<div class="crumbs">${out.join(' <span class="sep">/</span> ')}</div>`;
}

async function viewRepo(name) {
  const s = await api("GET", `/_vara/repositories/${encodeURIComponent(name)}/summary`);
  const last = s.last_commit;
  app.innerHTML = `
    <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>
    <p class="sub mono">${esc(s.id || "")}</p>
    ${repoTabs(name, "")}
    <div class="stats">
      <div class="stat"><div class="k">${ICON.branch(13)} Default branch</div><div class="v mono">${esc(s.default_branch || "—")}</div></div>
      <div class="stat"><div class="k">HEAD</div><div class="v mono">${esc(short(s.head)) || "—"}</div></div>
      <div class="stat"><div class="k">${ICON.commit(13)} Commits</div><div class="v">${s.commit_count}</div></div>
      <div class="stat"><div class="k">${ICON.branch(13)} Branches</div><div class="v">${s.branch_count}</div></div>
    </div>
    ${
      last
        ? `<h2>Latest commit</h2><div class="card">
             <div class="commit-msg">${esc(last.message.split("\n")[0])}</div>
             <div class="meta" style="margin-top:6px"><span class="commit-id">${esc(short(last.id))}</span> · ${esc(last.author)} · ${esc(when(last.timestamp))}</div>
           </div>`
        : `<div class="card empty">This repository is empty.</div>`
    }
    <div class="actions" style="margin-top:4px"><a class="btn" href="#/r/${encodeURIComponent(name)}/tree">Browse files</a></div>`;
}

async function viewHistory(name) {
  let before = "";
  const seen = [];
  const load = async () => {
    const q = `/_vara/repositories/${encodeURIComponent(name)}/commits?limit=30${before ? "&before=" + before : ""}`;
    const data = await api("GET", q);
    seen.push(...(data.commits || []));
    before = data.next || "";
    render();
  };
  const render = () => {
    const rows = seen
      .map(
        (c) => `<tr>
          <td><span class="commit-msg">${esc(c.message.split("\n")[0])}</span></td>
          <td><span class="chip mono">${esc(short(c.id))}</span></td>
          <td class="meta">${esc(c.author)}</td>
          <td class="meta">${esc(when(c.timestamp))}</td>
        </tr>`
      )
      .join("");
    app.innerHTML = `
      <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>${repoTabs(name, "history")}
      <div class="card pad0">
        <table><thead><tr><th>Message</th><th>Commit</th><th>Author</th><th>When</th></tr></thead>
        <tbody>${rows || `<tr><td colspan="4" class="empty">${ICON.commit(28)}No commits.</td></tr>`}</tbody></table>
      </div>
      ${before ? `<div class="actions"><button id="more">Load more</button></div>` : ""}`;
    const m = document.getElementById("more");
    if (m) m.onclick = () => guard(load);
  };
  await load();
}

async function viewBranches(name) {
  const data = await api("GET", `/_vara/repositories/${encodeURIComponent(name)}/branches`);
  const rows = (data.branches || [])
    .map(
      (b) => `<tr>
        <td><span class="rowlink"><span class="rowicon">${ICON.branch()}</span>${esc(b.name)}</span> ${b.is_head ? '<span class="chip head">HEAD</span>' : ""}</td>
        <td><span class="chip mono">${esc(short(b.target))}</span></td>
      </tr>`
    )
    .join("");
  app.innerHTML = `
    <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>${repoTabs(name, "branches")}
    <div class="card pad0"><table><thead><tr><th>Branch</th><th>Target</th></tr></thead>
    <tbody>${rows || `<tr><td colspan="2" class="empty">${ICON.branch(28)}No branches.</td></tr>`}</tbody></table></div>`;
}

async function viewSettings(name) {
  app.innerHTML = `
    <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>${repoTabs(name, "settings")}
    <div class="card">
      <label>Rename repository</label>
      <div class="actions" style="margin-top:4px">
        <input id="newname" value="${esc(name)}" style="flex:1" />
        <button class="primary" id="rename">Rename</button>
      </div>
    </div>
    <div class="card">
      <label>Danger zone</label>
      <p class="meta">Deleting a repository is permanent.</p>
      <div class="actions"><button class="danger" id="del">Delete repository</button></div>
    </div>`;
  document.getElementById("rename").onclick = async () => {
    const nn = document.getElementById("newname").value.trim();
    if (!nn || nn === name) return;
    try {
      await api("POST", `/_vara/repositories/${encodeURIComponent(name)}/rename`, { new_name: nn });
      toast("Renamed to " + nn);
      location.hash = "#/r/" + encodeURIComponent(nn) + "/settings";
    } catch (e) {
      toast(e.message, true);
    }
  };
  document.getElementById("del").onclick = async () => {
    if (!confirm(`Delete "${name}"? This cannot be undone.`)) return;
    try {
      await api("DELETE", `/_vara/repositories/${encodeURIComponent(name)}`);
      toast("Deleted " + name);
      location.hash = "#/";
    } catch (e) {
      toast(e.message, true);
    }
  };
}

// viewTree lists a directory (RFC-0022 tree endpoint). Directories sort before
// files, then by name. A directory links deeper; a file opens the blob view.
async function viewTree(name, path) {
  const url = `/_vara/repositories/${encodeURIComponent(name)}/tree/${encPath(path)}`;
  const data = await api("GET", url);
  const entries = (data.entries || []).slice().sort((a, b) =>
    a.type === b.type ? a.name.localeCompare(b.name) : a.type === "dir" ? -1 : 1
  );
  const rows = entries
    .map((e) => {
      const child = path ? path + "/" + e.name : e.name;
      const kind = e.type === "dir" ? "tree" : "blob";
      const href = `#/r/${encodeURIComponent(name)}/${kind}/${encodeURIComponent(child)}`;
      const icon = e.type === "dir" ? `<span class="rowicon dir">${ICON.folder()}</span>` : `<span class="rowicon">${ICON.file()}</span>`;
      return `<tr>
        <td><a class="rowlink" href="${href}">${icon}<span class="name">${esc(e.name)}</span></a></td>
        <td class="mono meta num">${esc(e.mode)}</td>
      </tr>`;
    })
    .join("");
  app.innerHTML = `
    <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>${repoTabs(name, "tree")}
    ${crumbs(name, path, false)}
    <div class="card pad0"><table><thead><tr><th>Name</th><th class="num">Mode</th></tr></thead>
    <tbody>${rows || `<tr><td colspan="2" class="empty">${ICON.folder(28)}Empty directory.</td></tr>`}</tbody></table></div>`;
}

// viewBlob shows a file (RFC-0022 blob endpoint). Text content is rendered as
// INERT text inside <pre> via esc() — never as HTML — the client-side mirror of
// the server's B5 guarantee. Binary or over-cap files link to the raw endpoint
// (which the server serves inert: text/plain or an attachment, always nosniff).
async function viewBlob(name, path) {
  const enc = encodeURIComponent(name);
  const data = await api("GET", `/_vara/repositories/${enc}/blob/${encPath(path)}`);
  const rawHref = `/_vara/repositories/${enc}/raw/${encPath(path)}`;
  const fname = esc(path.split("/").pop());
  let body;
  if (data.binary) {
    body = `<div class="empty">${ICON.binary(28)}Binary file (${data.size} bytes) — <a href="${rawHref}">download</a>.</div>`;
  } else if (data.truncated) {
    body = `<div class="empty">${ICON.file(28)}File too large to display inline (${data.size} bytes) — <a href="${rawHref}">download raw</a>.</div>`;
  } else {
    body = renderCode(data.content || "");
  }
  app.innerHTML = `
    <div class="page-head">${ICON.repo(20)}<h1>${esc(name)}</h1></div>${repoTabs(name, "tree")}
    ${crumbs(name, path, true)}
    <div class="card pad0">
      <div class="file-head">
        <span class="fname">${ICON.file(15)}${fname}</span>
        <span class="spacer"></span>
        <span class="fmeta">${data.size} bytes</span>
        <a class="chip" href="${rawHref}">Raw</a>
      </div>
      ${body}
    </div>`;
}

// --- router ------------------------------------------------------------------

function route() {
  const hash = location.hash || "#/";
  const parts = hash.slice(2).split("/").map(decodeURIComponent); // strip "#/"
  if (parts[0] === "login") return viewLogin();
  if (parts[0] === "r" && parts[1]) {
    const name = parts[1];
    const tab = parts[2] || "";
    if (tab === "tree") return guard(() => viewTree(name, parts[3] || ""));
    if (tab === "blob") return guard(() => viewBlob(name, parts[3] || ""));
    if (tab === "history") return guard(() => viewHistory(name));
    if (tab === "branches") return guard(() => viewBranches(name));
    if (tab === "settings") return guard(() => viewSettings(name));
    return guard(() => viewRepo(name));
  }
  return guard(viewDashboard);
}

window.addEventListener("hashchange", route);
(async function start() {
  await refreshWhoami();
  route();
})();
