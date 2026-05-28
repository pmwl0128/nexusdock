package httpx

import "net/http"

func (s *Server) uiIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uiHTML))
}

func (s *Server) uiApp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(uiJS))
}

const uiHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
  <title>MemoryDock</title>
  <style>
    :root {
      color-scheme: dark;
      --bg:#0d1117;
      --surface:#161b22;
      --surface-2:#0d1117;
      --surface-3:#21262d;
      --text:#e6edf3;
      --muted:#8b949e;
      --muted-2:#6e7681;
      --border:#30363d;
      --border-strong:#484f58;
      --accent:#58a6ff;
      --accent-2:#a371f7;
      --accent-soft:rgba(88,166,255,.14);
      --danger:#ff7b72;
      --danger-soft:rgba(248,81,73,.14);
      --ok:#56d364;
      --ok-soft:rgba(63,185,80,.14);
      --warning:#d29922;
      --radius:14px;
      --radius-lg:18px;
      --shadow:0 18px 48px rgba(0,0,0,.32);
      --mono:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono",monospace;
      --sans:Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    @media (prefers-color-scheme: light) {
      :root {
        color-scheme: light;
        --bg:#f6f8fa;
        --surface:#ffffff;
        --surface-2:#f6f8fa;
        --surface-3:#eef1f4;
        --text:#24292f;
        --muted:#57606a;
        --muted-2:#6e7781;
        --border:#d0d7de;
        --border-strong:#afb8c1;
        --accent:#0969da;
        --accent-2:#8250df;
        --accent-soft:rgba(9,105,218,.10);
        --danger:#cf222e;
        --danger-soft:rgba(207,34,46,.10);
        --ok:#1a7f37;
        --ok-soft:rgba(26,127,55,.10);
        --warning:#9a6700;
        --shadow:0 18px 42px rgba(27,31,36,.10);
      }
    }

    * { box-sizing:border-box; }
    html, body { min-height:100%; }
    body {
      margin:0; color:var(--text); background:var(--bg);
      font:14px/1.55 var(--sans); -webkit-font-smoothing:antialiased;
    }
    button, input, textarea { font:inherit; }
    button {
      min-height:32px; border:1px solid var(--border); background:var(--surface); color:var(--text);
      border-radius:9px; padding:6px 10px; cursor:pointer; transition:background .14s, border-color .14s, color .14s, transform .14s;
      display:inline-flex; align-items:center; justify-content:center; gap:7px; white-space:nowrap;
    }
    button:hover { border-color:var(--border-strong); background:var(--surface-3); }
    button:active { transform:translateY(1px); }
    button.primary { background:var(--accent); border-color:var(--accent); color:#fff; }
    button.primary:hover { filter:brightness(1.04); }
    button.danger { color:var(--danger); border-color:color-mix(in srgb, var(--danger) 35%, var(--border)); background:var(--danger-soft); }
    button:disabled { opacity:.45; cursor:not-allowed; transform:none; }
    input, textarea {
      width:100%; border:1px solid var(--border); border-radius:9px; padding:8px 10px; outline:none;
      background:var(--surface-2); color:var(--text); transition:border-color .14s, box-shadow .14s, background .14s;
    }
    input:focus, textarea:focus { border-color:var(--accent); box-shadow:0 0 0 3px var(--accent-soft); background:var(--surface); }
    textarea { min-height:58vh; resize:vertical; font-family:var(--mono); line-height:1.55; }
    pre { white-space:pre-wrap; word-break:break-word; margin:0; font-family:var(--mono); }
    h1, h2, h3, p { margin-top:0; }

    .app {
      min-height:100vh; display:grid; grid-template-columns:252px minmax(0,1fr);
    }
    .sidebar {
      position:sticky; top:0; height:100vh; padding:16px 14px; border-right:1px solid var(--border);
      background:var(--surface); display:flex; flex-direction:column; gap:16px;
    }
    .brand { display:flex; align-items:center; gap:11px; padding:4px 4px 12px; border-bottom:1px solid var(--border); }
    .brand-mark {
      width:38px; height:38px; border-radius:12px; display:grid; place-items:center; color:#fff; font-weight:800; letter-spacing:.2px;
      background:linear-gradient(135deg,var(--accent),var(--accent-2)); box-shadow:0 12px 28px rgba(88,166,255,.22);
    }
    .brand h1 { margin:0; font-size:17px; line-height:1.1; }
    .brand .subtitle { margin-top:3px; color:var(--muted); font-size:12px; }
    .tabs { display:flex; flex-direction:column; gap:6px; }
    .tabs button {
      width:100%; justify-content:flex-start; border-color:transparent; background:transparent; color:var(--muted); padding:9px 10px; border-radius:12px;
    }
    .tabs button::before { width:18px; text-align:center; color:var(--muted-2); }
    #tabMemories::before { content:"◫"; }
    #tabGit::before { content:"⑂"; }
    #tabConfig::before { content:"⚙"; }
    .tabs button:hover { color:var(--text); background:var(--surface-2); border-color:transparent; }
    .tabs button.primary { color:var(--text); background:var(--accent-soft); border-color:color-mix(in srgb, var(--accent) 34%, transparent); box-shadow:none; }
    .sidebar-card {
      margin-top:auto; border:1px solid var(--border); background:var(--surface-2); border-radius:16px; padding:12px;
      color:var(--muted); font-size:12px;
    }
    .sidebar-card strong { display:block; color:var(--text); margin-bottom:4px; font-size:13px; }

    .workspace { min-width:0; padding:18px; }
    .topbar {
      min-height:58px; display:flex; align-items:center; justify-content:space-between; gap:14px; margin-bottom:16px;
      border:1px solid var(--border); border-radius:var(--radius-lg); background:var(--surface); padding:12px 14px; box-shadow:var(--shadow);
    }
    .page-title { min-width:0; }
    .page-title h2 { margin:0; font-size:18px; letter-spacing:.1px; }
    .page-title p { margin:2px 0 0; color:var(--muted); font-size:12px; }
    .status-strip { display:flex; flex-wrap:wrap; gap:8px; justify-content:flex-end; }
    .pill {
      display:inline-flex; align-items:center; gap:6px; min-height:28px; padding:4px 9px; border:1px solid var(--border); border-radius:999px;
      background:var(--surface-2); color:var(--muted); font-size:12px;
    }
    .pill.ok { color:var(--ok); background:var(--ok-soft); border-color:color-mix(in srgb, var(--ok) 32%, var(--border)); }

    .layout { display:grid; grid-template-columns:minmax(300px, 380px) minmax(0,1fr); gap:16px; min-height:calc(100vh - 110px); }
    .panel, aside, main, .page-card {
      border:1px solid var(--border); border-radius:var(--radius-lg); background:var(--surface); box-shadow:var(--shadow); min-width:0;
    }
    aside { padding:0; overflow:hidden; display:flex; flex-direction:column; max-height:calc(100vh - 110px); position:sticky; top:18px; }
    main { padding:0; overflow:hidden; }
    .panel-head { padding:11px 12px; border-bottom:1px solid var(--border); display:flex; align-items:center; justify-content:space-between; gap:10px; }
    .panel-head h3 { margin:0; font-size:13px; letter-spacing:.08em; text-transform:uppercase; color:var(--muted); }
    .panel-body { padding:12px; min-width:0; }
    .explorer-body { overflow:auto; padding:8px; }
    .stack { display:flex; flex-direction:column; gap:12px; }
    .row { display:flex; gap:9px; align-items:center; }
    .search-grid { display:grid; grid-template-columns:minmax(0,1fr) auto; gap:8px; }
    .meta, .readonly { color:var(--muted); font-size:12px; }
    .path { font-family:var(--mono); font-size:12px; word-break:break-all; color:color-mix(in srgb, var(--text) 88%, var(--accent)); }
    .badge { display:inline-flex; align-items:center; gap:6px; border:1px solid var(--border); border-radius:999px; padding:4px 8px; background:var(--surface-2); color:var(--muted); font-size:12px; }

    .tree { display:flex; flex-direction:column; gap:1px; }
    .tree-row {
      width:100%; min-height:30px; display:grid; grid-template-columns:auto 14px minmax(0,1fr) 54px 24px 24px; align-items:center; gap:5px;
      text-align:left; border:1px solid transparent; border-radius:8px; padding:4px 6px; background:transparent; color:var(--text); box-shadow:none; cursor:pointer;
      font-size:13px; line-height:1.35;
    }
    .tree-row:hover { background:var(--surface-2); border-color:transparent; transform:none; }
    .tree-row.active { border-color:var(--accent); background:var(--accent-soft); }
    .tree-row.dragging { opacity:.48; }
    .tree-row.drop-target { border-color:var(--ok); background:var(--ok-soft); }
    .tree-row.file { cursor:grab; font-weight:420; }
    .tree-row.file:active { cursor:grabbing; }
    .tree-row.dir { font-weight:650; }
    .tree-indent { display:inline-block; width:0; height:1px; }
    .tree-toggle { width:14px; color:var(--muted); text-align:center; font-size:11px; }
    .tree-label { min-width:0; display:flex; align-items:center; gap:7px; overflow:hidden; }
    .tree-icon { flex:0 0 14px; width:14px; height:14px; position:relative; color:var(--muted); }
    .tree-icon.file::before { content:""; position:absolute; inset:1px 2px; border:1px solid var(--border-strong); border-radius:3px; background:var(--surface); }
    .tree-icon.file::after { content:""; position:absolute; right:2px; top:1px; width:5px; height:5px; border-left:1px solid var(--border-strong); border-bottom:1px solid var(--border-strong); background:var(--surface-2); }
    .tree-icon.folder::before { content:""; position:absolute; left:1px; right:1px; bottom:2px; height:10px; border:1px solid var(--border-strong); border-radius:3px; background:color-mix(in srgb, var(--accent) 10%, var(--surface-2)); }
    .tree-icon.folder::after { content:""; position:absolute; left:3px; top:1px; width:7px; height:4px; border:1px solid var(--border-strong); border-bottom:0; border-radius:3px 3px 0 0; background:color-mix(in srgb, var(--accent) 14%, var(--surface-2)); }
    .tree-name { min-width:0; display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .tree-meta { min-width:54px; color:var(--muted); font-size:11px; white-space:nowrap; text-align:right; overflow:hidden; text-overflow:ellipsis; }
    .tree-path { min-width:0; color:var(--muted); font-family:var(--mono); font-size:10px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .tree-action { min-height:22px; width:22px; padding:0; border-radius:7px; border-color:transparent; color:var(--muted); background:transparent; opacity:0; font-size:13px; }
    .tree-row:hover .tree-action, .tree-row.active .tree-action, .tree-row:focus-within .tree-action { opacity:1; }
    .tree-action:hover { color:var(--accent); border-color:var(--border); background:var(--accent-soft); transform:none; }
    .tree-action.delete:hover { color:var(--danger); background:var(--danger-soft); }

    .doc-toolbar { padding:14px; border-bottom:1px solid var(--border); display:flex; align-items:flex-start; justify-content:space-between; gap:14px; }
    .doc-head { min-width:0; }
    .toolbar-actions { display:flex; flex-wrap:wrap; gap:8px; justify-content:flex-end; }
    .card { background:var(--surface); padding:16px; }
    #viewer { min-height:calc(100vh - 182px); overflow:auto; }
    #contentView { font-size:13px; line-height:1.7; }
    .markdown-body { max-width:880px; margin:0 auto; color:var(--text); }
    .markdown-body.empty-doc { max-width:none; margin:0; color:var(--muted); }
    .markdown-body h1, .markdown-body h2, .markdown-body h3 { margin:1.1em 0 .45em; line-height:1.25; letter-spacing:-.02em; }
    .markdown-body h1 { font-size:26px; border-bottom:1px solid var(--border); padding-bottom:.35em; }
    .markdown-body h2 { font-size:20px; border-bottom:1px solid var(--border); padding-bottom:.3em; }
    .markdown-body h3 { font-size:16px; }
    .markdown-body p { margin:.7em 0; }
    .markdown-body ul, .markdown-body ol { margin:.7em 0; padding-left:1.45em; }
    .markdown-body li { margin:.22em 0; }
    .markdown-body blockquote { margin:1em 0; padding:.2em 1em; color:var(--muted); border-left:3px solid var(--border-strong); background:var(--surface-2); border-radius:0 8px 8px 0; }
    .markdown-body code { font-family:var(--mono); font-size:.92em; padding:.15em .35em; border-radius:6px; background:var(--surface-2); border:1px solid var(--border); }
    .markdown-body pre { margin:1em 0; padding:12px 14px; overflow:auto; border:1px solid var(--border); border-radius:12px; background:var(--surface-2); }
    .markdown-body pre code { padding:0; border:0; background:transparent; }
    .markdown-body a { color:var(--accent); text-decoration:none; }
    .markdown-body a:hover { text-decoration:underline; }
    .markdown-body hr { border:0; border-top:1px solid var(--border); margin:1.4em 0; }
    .markdown-body table { width:100%; border-collapse:collapse; margin:1em 0; display:block; overflow:auto; }
    .markdown-body th, .markdown-body td { border:1px solid var(--border); padding:6px 9px; }
    .markdown-body th { background:var(--surface-2); }
    .markdown-meta { margin:0 0 16px; border:1px solid var(--border); border-radius:12px; background:var(--surface-2); color:var(--muted); }
    .markdown-meta summary { cursor:pointer; padding:9px 12px; font-weight:650; color:var(--text); }
    .markdown-meta pre { margin:0; border:0; border-top:1px solid var(--border); border-radius:0 0 12px 12px; background:transparent; }
    #editor { padding:14px; background:var(--surface); }

    .page { display:block; }
    .git-grid { display:grid; grid-template-columns:minmax(0,1.38fr) minmax(320px,.62fr); gap:16px; }
    .sync-grid { display:grid; grid-template-columns:minmax(0,1fr) minmax(320px,.8fr); gap:16px; }
    .page-card { padding:0; overflow:hidden; }
    .card-title { padding:14px 16px; border-bottom:1px solid var(--border); display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
    .card-title h2 { margin:0; font-size:15px; letter-spacing:.1px; }
    .card-content { padding:14px 16px; }
    .diff-summary { display:grid; grid-template-columns:1fr; gap:10px; margin-bottom:12px; }
    #gitStatus, #gitStat, #syncStatus, #syncResult { border:1px solid var(--border); border-radius:12px; padding:11px; background:var(--surface-2); }
    #gitStatus { color:var(--ok); }
    #gitStat { color:var(--muted); }

    .diff-box { max-height:72vh; overflow:auto; border:1px solid var(--border); border-radius:12px; background:#1e1e1e; color:#d4d4d4; }
    .diff-view { min-width:980px; font:12px/1.48 var(--mono); }
    .diff-empty { padding:24px; color:#8b949e; text-align:center; }
    .diff-stage { position:sticky; top:0; z-index:3; padding:8px 12px; background:#252526; border-bottom:1px solid #3c3c3c; color:#c5c5c5; font-weight:700; }
    .diff-file { border-bottom:1px solid #3c3c3c; }
    .diff-file:last-child { border-bottom:0; }
    .diff-file-header { position:sticky; top:33px; z-index:2; display:flex; align-items:center; gap:8px; padding:7px 12px; background:#2d2d30; border-bottom:1px solid #3c3c3c; color:#e6edf3; }
    .diff-file-dot { width:8px; height:8px; border-radius:999px; background:#58a6ff; }
    .diff-file-name { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .diff-meta-line, .diff-hunk { display:grid; grid-template-columns:54px minmax(320px,1fr) 54px minmax(320px,1fr); min-height:22px; }
    .diff-meta-line { color:#8b949e; background:#252526; }
    .diff-hunk { color:#58a6ff; background:#26364a; }
    .diff-row { display:grid; grid-template-columns:54px minmax(320px,1fr) 54px minmax(320px,1fr); min-height:22px; }
    .diff-ln { user-select:none; text-align:right; padding:1px 10px; color:#858585; border-right:1px solid rgba(255,255,255,.08); background:rgba(255,255,255,.02); }
    .diff-code { white-space:pre; overflow:hidden; text-overflow:ellipsis; padding:1px 12px; border-right:1px solid rgba(255,255,255,.08); }
    .diff-right-code { border-right:0; }
    .diff-row.ctx { background:#1e1e1e; }
    .diff-row.add .diff-right-code, .diff-row.change .diff-right-code { background:rgba(46,160,67,.24); color:#b7f7c4; }
    .diff-row.del .diff-left-code, .diff-row.change .diff-left-code { background:rgba(248,81,73,.22); color:#ffd1d1; }
    .diff-row.note { color:#8b949e; background:#252526; font-style:italic; }
    .diff-spacer { background:#1b1b1b; color:#6e7681; }
    @media (prefers-color-scheme: light) {
      .diff-box { background:#fff; color:#24292f; }
      .diff-stage { background:#f6f8fa; border-bottom-color:#d0d7de; color:#57606a; }
      .diff-file-header { background:#f6f8fa; border-bottom-color:#d0d7de; color:#24292f; }
      .diff-meta-line { background:#f6f8fa; color:#57606a; }
      .diff-hunk { background:#ddf4ff; color:#0969da; }
      .diff-ln { color:#6e7781; border-right-color:rgba(27,31,36,.08); background:#f6f8fa; }
      .diff-code { border-right-color:rgba(27,31,36,.08); }
      .diff-row.ctx { background:#fff; }
      .diff-row.add .diff-right-code, .diff-row.change .diff-right-code { background:#dafbe1; color:#116329; }
      .diff-row.del .diff-left-code, .diff-row.change .diff-left-code { background:#ffebe9; color:#82071e; }
      .diff-row.note { background:#f6f8fa; color:#6e7781; }
      .diff-spacer { background:#f6f8fa; color:#8c959f; }
    }

    .commit-list { display:flex; flex-direction:column; gap:10px; max-height:68vh; overflow:auto; padding:14px 16px; }
    .commit { border:1px solid var(--border); border-radius:12px; background:var(--surface-2); padding:12px; }
    .commit-title { display:flex; gap:10px; align-items:baseline; justify-content:space-between; }
    .commit-title strong { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .hash { font-family:var(--mono); color:var(--accent); font-size:12px; }
    .empty { border:1px dashed var(--border-strong); border-radius:14px; padding:18px; color:var(--muted); text-align:center; background:var(--surface-2); }
    .toast { position:fixed; right:18px; bottom:18px; max-width:min(520px, calc(100vw - 36px)); border:1px solid var(--border); border-radius:14px; padding:11px 13px; background:var(--surface); box-shadow:var(--shadow); display:none; z-index:60; }
    .ok { color:var(--ok); } .dangerText { color:var(--danger); }
    .hidden { display:none !important; }

    @media (max-width: 1080px) {
      .app { grid-template-columns:1fr; }
      .sidebar { position:static; height:auto; padding:10px 12px; border-right:0; border-bottom:1px solid var(--border); flex-direction:row; align-items:center; gap:12px; }
      .brand { border-bottom:0; padding:0; flex:0 0 auto; }
      .tabs { flex:1; flex-direction:row; overflow:auto; }
      .tabs button { width:auto; flex:1 0 auto; justify-content:center; }
      .sidebar-card { display:none; }
      .workspace { padding:12px; }
      .topbar { margin-bottom:12px; }
      .git-grid, .sync-grid { grid-template-columns:1fr; }
      .commit-list { max-height:none; }
    }
    @media (max-width: 760px) {
      .brand .subtitle, .status-strip { display:none; }
      .brand-mark { width:34px; height:34px; border-radius:10px; }
      .workspace { padding:10px; }
      .topbar { border-radius:14px; padding:12px; }
      .layout { grid-template-columns:1fr; min-height:auto; gap:10px; }
      aside { position:static; max-height:none; border-radius:14px; }
      main { border-radius:14px; }
      .search-grid { grid-template-columns:1fr; }
      .doc-toolbar, .card-title { flex-direction:column; align-items:stretch; }
      .toolbar-actions { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); }
      .toolbar-actions button { width:100%; }
      #viewer { min-height:44vh; }
      textarea { min-height:46vh; }
      .diff-box { max-height:56vh; }
      .diff-view { min-width:860px; }
      .commit-title { align-items:flex-start; }
      .commit-title strong { white-space:normal; }
    }
    @media (max-width: 460px) {
      body { font-size:13px; }
      .sidebar { padding:9px; }
      .brand h1 { font-size:15px; }
      .tabs button { min-width:max-content; padding:8px 9px; }
      .tree-row { grid-template-columns:auto 14px minmax(0,1fr) 22px 22px; }
      .tree-row .tree-meta { display:none; }
      .tree-indent { max-width:40px; }
      #gitLogLimit { width:100% !important; }
      .row { flex-wrap:wrap; align-items:stretch; }
      .row input { min-width:0; }
      .row button { flex:1; }
      .toast { right:10px; bottom:10px; }
    }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark">M</div>
        <div>
          <h1>MemoryDock</h1>
          <div class="subtitle">Knowledge workspace</div>
        </div>
      </div>
      <nav class="tabs" aria-label="MemoryDock navigation">
        <button id="tabMemories" class="primary">记忆库</button>
        <button id="tabGit">变更记录</button>
        <button id="tabConfig">同步设置</button>
      </nav>
      <div class="sidebar-card">
        <strong>Git backed memory</strong>
        本地 Markdown 记忆库，支持目录整理、拖拽移动、Diff 审阅与同步。
      </div>
    </aside>

    <section class="workspace">
      <div class="topbar">
        <div class="page-title">
          <h2>Memory workspace</h2>
          <p>浏览、整理、编辑和审阅你的记忆文件。</p>
        </div>
        <div class="status-strip">
          <span class="pill ok">● Local service online</span>
          <span class="pill">Markdown · Git · Sync</span>
        </div>
      </div>

      <section id="memoriesView" class="layout">
        <aside>
          <div class="panel-head">
            <h3>Explorer</h3>
            <span class="badge" id="listMeta">加载中…</span>
          </div>
          <div class="panel-body stack">
            <div class="search-grid">
              <input id="searchInput" placeholder="搜索关键词，例如 origin credential" />
              <button id="searchBtn" class="primary">搜索</button>
            </div>
            <div class="search-grid">
              <input id="prefixInput" placeholder="prefix，可选，例如 shared/projects" />
              <button id="refreshBtn">刷新</button>
            </div>
          </div>
          <div class="explorer-body">
            <div class="tree" id="memoryList"></div>
          </div>
        </aside>
        <main>
          <div class="doc-toolbar">
            <div class="doc-head">
              <div class="path" id="currentPath">未选择文件</div>
              <div class="readonly" id="modeText">默认只读。点击“编辑”后才可修改。</div>
            </div>
            <div class="toolbar-actions">
              <button id="newBtn">新建</button>
              <button id="editBtn" disabled>编辑</button>
              <button id="saveBtn" class="primary hidden">保存</button>
              <button id="cancelBtn" class="hidden">取消</button>
              <button id="deleteBtn" class="danger" disabled>删除</button>
            </div>
          </div>
          <div id="viewer" class="card"><div id="contentView" class="markdown-body empty-doc">从左侧选择一个记忆文件。</div></div>
          <div id="editor" class="hidden stack">
            <input id="pathInput" placeholder="memory-relative path，例如 inbox/note.md" />
            <textarea id="contentEdit" spellcheck="false"></textarea>
          </div>
        </main>
      </section>

      <section id="gitView" class="hidden page">
        <div class="git-grid">
          <div class="page-card stack">
            <div class="card-title">
              <div>
                <h2>Git Diff</h2>
                <div class="meta" id="gitDiffMeta">查看 memory 仓库当前未提交更改。</div>
              </div>
              <button id="gitDiffBtn" class="primary">刷新 Diff</button>
            </div>
            <div class="card-content">
              <div class="diff-summary">
                <pre id="gitStatus">未加载</pre>
                <pre id="gitStat" class="meta"></pre>
              </div>
              <div class="diff-box"><div id="gitDiff" class="diff-view"><div class="diff-empty">未加载</div></div></div>
            </div>
          </div>
          <div class="page-card">
            <div class="card-title">
              <div>
                <h2>提交历史</h2>
                <div class="meta">最近 Git commit，方便快速查阅同步记录。</div>
              </div>
              <div class="row">
                <input id="gitLogLimit" value="50" style="width:90px" inputmode="numeric" />
                <button id="gitLogBtn">刷新</button>
              </div>
            </div>
            <div id="gitLog" class="commit-list"><div class="empty">未加载</div></div>
          </div>
        </div>
      </section>

      <section id="configView" class="hidden page">
        <div class="sync-grid">
          <div class="page-card stack">
            <div class="card-title">
              <div>
                <h2>同步状态</h2>
                <div class="meta">查看 Git 仓库、ahead/behind、dirty 与最近同步状态。</div>
              </div>
              <button id="syncStatusBtn" class="primary">刷新状态</button>
            </div>
            <div class="card-content"><pre id="syncStatus">未加载</pre></div>
          </div>
          <div class="page-card stack">
            <div class="card-title">
              <div>
                <h2>手动同步到 Git</h2>
                <div class="meta">这些按钮调用 MemoryDock 自己的同步 API。自动同步仍由后台配置控制。</div>
              </div>
            </div>
            <div class="card-content stack">
              <div class="toolbar-actions">
                <button id="pullBtn">Pull</button>
                <button id="pushBtn">Push</button>
                <button id="syncNowBtn" class="primary">Pull + Push</button>
              </div>
              <pre id="syncResult">等待操作</pre>
            </div>
          </div>
        </div>
      </section>
    </section>
  </div>
  <div class="toast" id="toast"></div>
  <script src="/ui/app.js"></script>
</body>
</html>`

const uiJS = `
const state = { currentPath: '', currentContent: '', editing: false, entries: [], expanded: new Set(['']), draggingPath: '' };
const $ = (id) => document.getElementById(id);

function toast(message, danger=false) {
  const el = $('toast');
  el.textContent = message;
  el.className = 'toast ' + (danger ? 'dangerText' : 'ok');
  el.style.display = 'block';
  setTimeout(() => { el.style.display = 'none'; }, 3200);
}

async function api(path, options={}) {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...options });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.ok === false) throw new Error(data?.error?.message || res.statusText);
  return data;
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, ch => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[ch]));
}

function markdownLinkify(text) {
  let html = escapeHTML(text);
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  html = html.replace(new RegExp(String.fromCharCode(96) + '([^' + String.fromCharCode(96) + ']+)' + String.fromCharCode(96), 'g'), '<code>$1</code>');
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  return html;
}

function splitFrontmatter(content) {
  if (!content.startsWith('---\n')) return { meta: '', body: content };
  const end = content.indexOf('\n---\n', 4);
  if (end < 0) return { meta: '', body: content };
  return { meta: content.slice(4, end), body: content.slice(end + 5) };
}

function renderMarkdown(content) {
  const parts = splitFrontmatter(content);
  const lines = parts.body.replace(/\r\n/g, '\n').split('\n');
  const out = [];
  let paragraph = [];
  let list = null;
  let code = null;
  let quote = [];

  const flushParagraph = () => {
    if (!paragraph.length) return;
    out.push('<p>' + markdownLinkify(paragraph.join(' ')) + '</p>');
    paragraph = [];
  };
  const flushList = () => {
    if (!list) return;
    out.push('<' + list.type + '>' + list.items.map(item => '<li>' + markdownLinkify(item) + '</li>').join('') + '</' + list.type + '>');
    list = null;
  };
  const flushQuote = () => {
    if (!quote.length) return;
    out.push('<blockquote>' + quote.map(line => '<p>' + markdownLinkify(line) + '</p>').join('') + '</blockquote>');
    quote = [];
  };
  const closeBlocks = () => { flushParagraph(); flushList(); flushQuote(); };

  for (const line of lines) {
    if (line.startsWith(String.fromCharCode(96,96,96))) {
      if (code) {
        out.push('<pre><code>' + escapeHTML(code.lines.join('\n')) + '</code></pre>');
        code = null;
      } else {
        closeBlocks();
        code = { lines: [] };
      }
      continue;
    }
    if (code) {
      code.lines.push(line);
      continue;
    }
    if (!line.trim()) {
      closeBlocks();
      continue;
    }
    if (/^---+$/.test(line.trim())) {
      closeBlocks();
      out.push('<hr />');
      continue;
    }
    const heading = /^(#{1,6})\s+(.+)$/.exec(line);
    if (heading) {
      closeBlocks();
      const level = Math.min(6, heading[1].length);
      out.push('<h' + level + '>' + markdownLinkify(heading[2].trim()) + '</h' + level + '>');
      continue;
    }
    if (line.startsWith('>')) {
      flushParagraph();
      flushList();
      quote.push(line.replace(/^>\s?/, ''));
      continue;
    }
    const unordered = /^\s*[-*+]\s+(.+)$/.exec(line);
    const ordered = /^\s*\d+[.)]\s+(.+)$/.exec(line);
    if (unordered || ordered) {
      flushParagraph();
      flushQuote();
      const type = unordered ? 'ul' : 'ol';
      if (!list || list.type !== type) flushList();
      if (!list) list = { type, items: [] };
      list.items.push((unordered || ordered)[1]);
      continue;
    }
    paragraph.push(line.trim());
  }
  if (code) out.push('<pre><code>' + escapeHTML(code.lines.join('\n')) + '</code></pre>');
  closeBlocks();
  const meta = parts.meta.trim()
    ? '<details class="markdown-meta"><summary>Frontmatter</summary><pre><code>' + escapeHTML(parts.meta.trim()) + '</code></pre></details>'
    : '';
  return meta + (out.join('\n') || '<p class="meta">空 Markdown 文件</p>');
}

function renderMemoryContent(path, content) {
  const viewer = $('contentView');
  viewer.className = '';
  if (/\.(md|markdown)$/i.test(path)) {
    viewer.className = 'markdown-body';
    viewer.innerHTML = renderMarkdown(content);
  } else {
    viewer.className = '';
    const pre = document.createElement('pre');
    pre.textContent = content;
    viewer.innerHTML = '';
    viewer.appendChild(pre);
  }
}

function setTab(tab) {
  const isMem = tab === 'memories';
  const isGit = tab === 'git';
  const isConfig = tab === 'config';
  $('memoriesView').classList.toggle('hidden', !isMem);
  $('gitView').classList.toggle('hidden', !isGit);
  $('configView').classList.toggle('hidden', !isConfig);
  $('tabMemories').classList.toggle('primary', isMem);
  $('tabGit').classList.toggle('primary', isGit);
  $('tabConfig').classList.toggle('primary', isConfig);
  if (isGit) loadGitPanel();
  if (isConfig) loadSyncStatus();
}

function setEditing(editing) {
  state.editing = editing;
  $('viewer').classList.toggle('hidden', editing);
  $('editor').classList.toggle('hidden', !editing);
  $('saveBtn').classList.toggle('hidden', !editing);
  $('cancelBtn').classList.toggle('hidden', !editing);
  $('editBtn').classList.toggle('hidden', editing);
  $('modeText').textContent = editing ? '编辑模式。保存会覆盖当前文件。' : '默认只读。点击“编辑”后才可修改。';
  if (editing) {
    $('pathInput').value = state.currentPath;
    $('contentEdit').value = state.currentContent;
  }
}

function normalizePath(path) {
  return String(path || '').replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function bytesText(bytes) {
  if (!bytes) return '';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1024 / 1024).toFixed(1) + ' MB';
}

function fileNameFromPath(path) {
  const parts = normalizePath(path).split('/').filter(Boolean);
  return parts[parts.length - 1] || '';
}

function joinMemoryPath(dir, name) {
  dir = normalizePath(dir);
  name = fileNameFromPath(name);
  return dir ? dir + '/' + name : name;
}

function parentPath(path) {
  const parts = normalizePath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function replacePathPrefix(path, fromPath, toPath) {
  path = normalizePath(path);
  fromPath = normalizePath(fromPath);
  toPath = normalizePath(toPath);
  if (path === fromPath) return toPath;
  if (fromPath && path.startsWith(fromPath + '/')) return toPath + path.slice(fromPath.length);
  return path;
}

function rewriteExpandedPaths(fromPath, toPath) {
  const rewritten = new Set(['']);
  for (const path of state.expanded) {
    if (!path) continue;
    rewritten.add(replacePathPrefix(path, fromPath, toPath));
  }
  state.expanded = rewritten;
}

function isPathInside(path, dirPath) {
  path = normalizePath(path);
  dirPath = normalizePath(dirPath);
  return Boolean(path && dirPath && (path === dirPath || path.startsWith(dirPath + '/')));
}

function clearCurrentMemory() {
  state.currentPath = '';
  state.currentContent = '';
  $('currentPath').textContent = '未选择文件';
  $('contentView').className = 'markdown-body empty-doc';
  $('contentView').textContent = '从左侧选择一个记忆文件。';
  $('editBtn').disabled = true;
  $('deleteBtn').disabled = true;
  setEditing(false);
}

function newDirNode(name, path) {
  return { name, path, type: 'directory', children: new Map(), entry: null };
}

function buildTree(entries) {
  const root = newDirNode('', '');
  for (const entry of entries) {
    const fullPath = normalizePath(entry.path);
    if (!fullPath) continue;
    const parts = fullPath.split('/').filter(Boolean);
    let node = root;
    let cursor = '';
    for (let i = 0; i < parts.length; i++) {
      const name = parts[i];
      cursor = cursor ? cursor + '/' + name : name;
      const isLeaf = i === parts.length - 1;
      let child = node.children.get(name);
      if (!child) {
        child = newDirNode(name, cursor);
        node.children.set(name, child);
      }
      if (isLeaf) {
        child.entry = entry;
        child.type = entry.type === 'file' ? 'file' : 'directory';
      }
      node = child;
    }
  }
  return root;
}

function sortedChildren(node) {
  return [...node.children.values()].sort((a, b) => {
    if (a.type !== b.type) return a.type === 'directory' ? -1 : 1;
    return a.name.localeCompare(b.name, 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
  });
}

function countFiles(node) {
  if (node.type === 'file') return 1;
  let count = 0;
  for (const child of node.children.values()) count += countFiles(child);
  return count;
}

function expandPath(path) {
  const parts = normalizePath(path).split('/').filter(Boolean);
  let cursor = '';
  for (let i = 0; i < parts.length - 1; i++) {
    cursor = cursor ? cursor + '/' + parts[i] : parts[i];
    state.expanded.add(cursor);
  }
}

function expandAllDirs(node) {
  if (node.type !== 'directory') return;
  state.expanded.add(node.path);
  for (const child of node.children.values()) expandAllDirs(child);
}

function renderTree(entries, options={}) {
  const list = $('memoryList');
  list.innerHTML = '';
  if (!entries.length) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = options.emptyText || '没有条目';
    list.appendChild(empty);
    return;
  }

  const root = buildTree(entries);
  if (options.expandAll) expandAllDirs(root);
  if (state.currentPath) expandPath(state.currentPath);

  for (const child of sortedChildren(root)) renderTreeNode(child, 0, options);
}

function renderTreeNode(node, depth, options={}) {
  const list = $('memoryList');
  const isDir = node.type === 'directory';
  const isOpen = state.expanded.has(node.path);
  const row = document.createElement('div');
  row.tabIndex = 0;
  row.role = 'button';
  row.className = 'tree-row ' + (isDir ? 'dir' : 'file') + (node.path === state.currentPath ? ' active' : '');
  row.title = node.path;

  const indent = document.createElement('span');
  indent.className = 'tree-indent';
  indent.style.width = String(depth * 16) + 'px';
  row.appendChild(indent);

  const toggle = document.createElement('span');
  toggle.className = 'tree-toggle';
  toggle.textContent = isDir ? (isOpen ? '▾' : '▸') : '·';
  row.appendChild(toggle);

  const nameWrap = document.createElement('span');
  nameWrap.className = 'tree-label';
  const icon = document.createElement('span');
  icon.className = 'tree-icon ' + (isDir ? 'folder' : 'file');
  nameWrap.appendChild(icon);
  const name = document.createElement('span');
  name.className = 'tree-name';
  name.textContent = node.name;
  nameWrap.appendChild(name);
  if (options.showPath && !isDir) {
    const path = document.createElement('div');
    path.className = 'tree-path';
    path.textContent = node.path;
    nameWrap.appendChild(path);
  }
  row.appendChild(nameWrap);

  const meta = document.createElement('span');
  meta.className = 'tree-meta';
  meta.textContent = isDir ? countFiles(node) + ' 文件' : bytesText(node.entry?.size_bytes);
  row.appendChild(meta);

  const renameBtn = document.createElement('button');
  renameBtn.type = 'button';
  renameBtn.className = 'tree-action';
  renameBtn.title = isDir ? '重命名文件夹' : '重命名文件';
  renameBtn.textContent = '✎';
  renameBtn.onclick = (event) => {
    event.preventDefault();
    event.stopPropagation();
    renameNode(node, isDir).catch(e => toast(e.message, true));
  };
  row.appendChild(renameBtn);

  const deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.className = 'tree-action delete';
  deleteBtn.title = isDir ? '删除文件夹' : '删除文件';
  deleteBtn.textContent = '×';
  deleteBtn.onclick = (event) => {
    event.preventDefault();
    event.stopPropagation();
    deleteNode(node, isDir).catch(e => toast(e.message, true));
  };
  row.appendChild(deleteBtn);

  const activateRow = () => {
    if (isDir) {
      if (state.expanded.has(node.path)) state.expanded.delete(node.path);
      else state.expanded.add(node.path);
      renderTree(options.entries || state.entries, options);
      return;
    }
    loadMemory(node.path);
  };
  row.onclick = activateRow;
  row.onkeydown = (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      activateRow();
    }
  };

  if (!isDir) {
    row.draggable = true;
    row.addEventListener('dragstart', (event) => {
      state.draggingPath = node.path;
      row.classList.add('dragging');
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', node.path);
      event.dataTransfer.setData('application/x-memorydock-path', node.path);
    });
    row.addEventListener('dragend', () => {
      state.draggingPath = '';
      row.classList.remove('dragging');
      document.querySelectorAll('.tree-row.drop-target').forEach(el => el.classList.remove('drop-target'));
    });
  } else {
    row.addEventListener('dragenter', (event) => {
      if (!state.draggingPath) return;
      event.preventDefault();
      row.classList.add('drop-target');
    });
    row.addEventListener('dragover', (event) => {
      if (!state.draggingPath) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = 'move';
      row.classList.add('drop-target');
    });
    row.addEventListener('dragleave', () => {
      row.classList.remove('drop-target');
    });
    row.addEventListener('drop', (event) => {
      event.preventDefault();
      row.classList.remove('drop-target');
      const fromPath = event.dataTransfer.getData('application/x-memorydock-path') || event.dataTransfer.getData('text/plain') || state.draggingPath;
      moveMemoryToDirectory(fromPath, node.path).catch(e => toast(e.message, true));
    });
  }
  list.appendChild(row);

  if (isDir && isOpen) {
    for (const child of sortedChildren(node)) renderTreeNode(child, depth + 1, options);
  }
}

async function loadList() {
  const prefix = encodeURIComponent($('prefixInput').value.trim());
  const data = await api('/v1/memories?max_entries=500' + (prefix ? '&prefix=' + prefix : ''));
  state.entries = data.entries || [];
  renderTree(state.entries, { entries: state.entries, expandAll: true });
  const fileCount = state.entries.filter(e => e.type === 'file').length;
  const dirCount = state.entries.filter(e => e.type === 'directory').length;
  $('listMeta').textContent = data.count + ' 个条目 · ' + dirCount + ' 个目录 · ' + fileCount + ' 个文件';
}

async function doSearch() {
  const query = $('searchInput').value.trim();
  if (!query) return loadList();
  const prefix = $('prefixInput').value.trim();
  const data = await api('/v1/memories/search', { method: 'POST', body: JSON.stringify({ query, prefix, max_results: 100 }) });
  const entries = (data.results || []).map(result => ({
    path: result.path,
    name: result.path.split('/').pop(),
    type: 'file',
    size_bytes: result.size_bytes || 0,
    search_title: result.title,
    matched_terms: result.matched_terms || []
  }));
  renderTree(entries, { entries, expandAll: true, showPath: true, emptyText: '没有搜索结果' });
  $('listMeta').textContent = data.count + ' 个搜索结果 · 按目录分组展示';
}

async function loadMemory(path) {
  const data = await api('/v1/memories/' + encodeURIComponent(path));
  state.currentPath = data.memory.path;
  state.currentContent = data.memory.content;
  $('currentPath').textContent = state.currentPath;
  renderMemoryContent(state.currentPath, state.currentContent);
  $('editBtn').disabled = false;
  $('deleteBtn').disabled = false;
  expandPath(state.currentPath);
  setEditing(false);
  renderTree(state.entries, { entries: state.entries });
}

async function moveMemoryToDirectory(fromPath, dirPath) {
  fromPath = normalizePath(fromPath);
  dirPath = normalizePath(dirPath);
  if (!fromPath) return;
  const toPath = joinMemoryPath(dirPath, fromPath);
  if (!toPath || toPath === fromPath) return toast('文件已经在该目录中', true);
  if (!confirm('移动文件？\n\n' + fromPath + '\n→ ' + toPath)) return;
  const data = await api('/v1/memories/move', {
    method: 'POST',
    body: JSON.stringify({ from_path: fromPath, to_path: toPath, confirmed: true, overwrite: false })
  });
  toast('已移动到 ' + toPath);
  state.currentPath = data.memory.path;
  state.currentContent = data.memory.content;
  expandPath(state.currentPath);
  await loadList();
  await loadMemory(data.memory.path);
}

async function renameNode(node, isDir) {
  const oldPath = normalizePath(node.path);
  const oldName = fileNameFromPath(oldPath);
  const newName = prompt(isDir ? '新的文件夹名称' : '新的文件名称', oldName);
  if (newName === null) return;
  const trimmed = newName.trim();
  if (!trimmed || trimmed.includes('/') || trimmed.includes('\\') || trimmed.startsWith('.')) {
    return toast('名称不能为空，且不能包含 /、\\ 或以 . 开头', true);
  }
  const newPath = joinMemoryPath(parentPath(oldPath), trimmed);
  if (!newPath || newPath === oldPath) return;
  if (!isDir && !/\.(md|markdown|txt)$/i.test(newPath)) {
    return toast('文件名需要以 .md、.markdown 或 .txt 结尾', true);
  }
  if (!confirm((isDir ? '重命名文件夹？' : '重命名文件？') + '\n\n' + oldPath + '\n→ ' + newPath)) return;
  await api('/v1/memories/move', {
    method: 'POST',
    body: JSON.stringify({ from_path: oldPath, to_path: newPath, confirmed: true, overwrite: false })
  });
  toast('已重命名为 ' + newPath);
  rewriteExpandedPaths(oldPath, newPath);
  const nextCurrentPath = replacePathPrefix(state.currentPath, oldPath, newPath);
  await loadList();
  if (!isDir || nextCurrentPath !== state.currentPath) {
    await loadMemory(nextCurrentPath);
  } else {
    renderTree(state.entries, { entries: state.entries });
  }
}

async function deleteNode(node, isDir) {
  const path = normalizePath(node.path);
  if (!path) return;
  const message = isDir
    ? '确认递归删除整个文件夹？\n\n' + path + '\n\n其中的所有文件都会被删除。'
    : '确认删除文件？\n\n' + path;
  if (!confirm(message)) return;
  await api('/v1/memories/' + encodeURIComponent(path) + '?confirmed=true', { method: 'DELETE' });
  toast(isDir ? '已删除文件夹 ' + path : '已删除文件 ' + path);
  if (isDir ? isPathInside(state.currentPath, path) : state.currentPath === path) {
    clearCurrentMemory();
  }
  state.expanded.delete(path);
  await loadList();
}


function newMemory() {
  state.currentPath = '';
  state.currentContent = '---\ntype: note\nscope: inbox\nsource: user-confirmed\nconfidence: medium\n---\n\n# 新记忆\n\n';
  $('currentPath').textContent = '新建记忆';
  $('editBtn').disabled = true;
  $('deleteBtn').disabled = true;
  setEditing(true);
}

async function saveMemory() {
  const path = $('pathInput').value.trim();
  const content = $('contentEdit').value;
  if (!path || !content.trim()) return toast('path 和 content 不能为空', true);
  const isExisting = Boolean(state.currentPath);
  const target = isExisting ? '/v1/memories/' + encodeURIComponent(state.currentPath) : '/v1/memories';
  const body = isExisting ? { path: state.currentPath, content, confirmed: true, overwrite: true } : { path, content, confirmed: true, overwrite: true };
  const data = await api(target, { method: isExisting ? 'PATCH' : 'POST', body: JSON.stringify(body) });
  toast('已保存');
  await loadList();
  await loadMemory(data.memory.path);
}

async function deleteMemory() {
  if (!state.currentPath) return;
  if (!confirm('确认删除：' + state.currentPath + ' ?')) return;
  await api('/v1/memories/' + encodeURIComponent(state.currentPath) + '?confirmed=true', { method: 'DELETE' });
  toast('已删除');
  clearCurrentMemory();
  await loadList();
}

async function loadGitPanel() {
  await Promise.all([
    loadGitDiff().catch(e => toast(e.message, true)),
    loadGitLog().catch(e => toast(e.message, true))
  ]);
}

function parseHunkHeader(line) {
  const match = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(line);
  return {
    oldLine: match ? Number(match[1]) : 0,
    newLine: match ? Number(match[3]) : 0
  };
}

function diffFileName(line) {
  const parts = line.trim().split(/\s+/);
  const b = parts.find(part => part.startsWith('b/'));
  const a = parts.find(part => part.startsWith('a/'));
  return (b || a || parts[parts.length - 1] || 'diff').replace(/^[ab]\//, '');
}

function appendSideBySideLine(parent, className, leftNo, leftText, rightNo, rightText) {
  const row = document.createElement('div');
  row.className = 'diff-row ' + className;
  const oldCell = document.createElement('span');
  oldCell.className = 'diff-ln';
  oldCell.textContent = leftNo ? String(leftNo) : '';
  const leftCode = document.createElement('span');
  leftCode.className = 'diff-code diff-left-code' + (!leftText ? ' diff-spacer' : '');
  leftCode.textContent = leftText || ' ';
  const newCell = document.createElement('span');
  newCell.className = 'diff-ln';
  newCell.textContent = rightNo ? String(rightNo) : '';
  const rightCode = document.createElement('span');
  rightCode.className = 'diff-code diff-right-code' + (!rightText ? ' diff-spacer' : '');
  rightCode.textContent = rightText || ' ';
  row.appendChild(oldCell);
  row.appendChild(leftCode);
  row.appendChild(newCell);
  row.appendChild(rightCode);
  parent.appendChild(row);
}

function appendSideBySideMeta(parent, className, text) {
  const row = document.createElement('div');
  row.className = className;
  const oldCell = document.createElement('span');
  oldCell.className = 'diff-ln';
  const left = document.createElement('span');
  left.className = 'diff-code';
  left.textContent = text;
  const newCell = document.createElement('span');
  newCell.className = 'diff-ln';
  const right = document.createElement('span');
  right.className = 'diff-code diff-right-code';
  right.textContent = text;
  row.appendChild(oldCell);
  row.appendChild(left);
  row.appendChild(newCell);
  row.appendChild(right);
  parent.appendChild(row);
}

function renderUnifiedDiff(container, sections) {
  container.innerHTML = '';
  const nonEmpty = sections.filter(section => section.diff && section.diff.trim());
  if (!nonEmpty.length) {
    const empty = document.createElement('div');
    empty.className = 'diff-empty';
    empty.textContent = '没有 diff。';
    container.appendChild(empty);
    return;
  }

  for (const section of nonEmpty) {
    const stage = document.createElement('div');
    stage.className = 'diff-stage';
    stage.textContent = section.title;
    container.appendChild(stage);

    let fileEl = null;
    let bodyEl = null;
    let oldLine = 0;
    let newLine = 0;
    let pendingDeletes = [];

    const flushDeletes = () => {
      for (const item of pendingDeletes) appendSideBySideLine(bodyEl, 'del', item.no, item.text, '', '');
      pendingDeletes = [];
    };
    const ensureFile = (name='diff') => {
      if (fileEl) return;
      fileEl = document.createElement('div');
      fileEl.className = 'diff-file';
      const header = document.createElement('div');
      header.className = 'diff-file-header';
      const dot = document.createElement('span');
      dot.className = 'diff-file-dot';
      const label = document.createElement('span');
      label.className = 'diff-file-name';
      label.textContent = name;
      header.appendChild(dot);
      header.appendChild(label);
      bodyEl = document.createElement('div');
      fileEl.appendChild(header);
      fileEl.appendChild(bodyEl);
      container.appendChild(fileEl);
    };

    for (const line of section.diff.split('\n')) {
      if (line.startsWith('diff --git ')) {
        if (bodyEl) flushDeletes();
        fileEl = null;
        bodyEl = null;
        ensureFile(diffFileName(line));
        continue;
      }
      ensureFile();
      if (line.startsWith('@@ ')) {
        flushDeletes();
        const parsed = parseHunkHeader(line);
        oldLine = parsed.oldLine;
        newLine = parsed.newLine;
        appendSideBySideMeta(bodyEl, 'diff-hunk', line);
        continue;
      }
      if (line.startsWith('index ') || line.startsWith('new file mode') || line.startsWith('deleted file mode') || line.startsWith('similarity index') || line.startsWith('rename from') || line.startsWith('rename to') || line.startsWith('--- ') || line.startsWith('+++ ')) {
        flushDeletes();
        appendSideBySideMeta(bodyEl, 'diff-meta-line', line);
        continue;
      }
      if (line.startsWith('\\ No newline')) {
        flushDeletes();
        appendSideBySideLine(bodyEl, 'note', '', line, '', line);
        continue;
      }
      if (line.startsWith('-')) {
        pendingDeletes.push({ no: oldLine++, text: line });
        continue;
      }
      if (line.startsWith('+')) {
        const deleted = pendingDeletes.shift();
        if (deleted) appendSideBySideLine(bodyEl, 'change', deleted.no, deleted.text, newLine++, line);
        else appendSideBySideLine(bodyEl, 'add', '', '', newLine++, line);
        continue;
      }
      flushDeletes();
      appendSideBySideLine(bodyEl, 'ctx', oldLine ? oldLine++ : '', line || ' ', newLine ? newLine++ : '', line || ' ');
    }
    if (bodyEl) flushDeletes();
  }
}

async function loadGitDiff() {
  $('gitStatus').textContent = '加载中…';
  $('gitStat').textContent = '';
  renderUnifiedDiff($('gitDiff'), []);
  const data = await api('/v1/git/diff');
  if (!data.git_repo) {
    $('gitStatus').textContent = '当前 memory 目录不是 Git 仓库。';
    renderUnifiedDiff($('gitDiff'), []);
    return;
  }
  $('gitDiffMeta').textContent = data.dirty ? '有未提交更改' : '工作区干净，没有未提交更改';
  $('gitStatus').textContent = data.status || '工作区干净';
  $('gitStat').textContent = data.stat || '';
  renderUnifiedDiff($('gitDiff'), [
    { title: 'Staged changes', diff: data.cached_diff || '' },
    { title: 'Working tree changes', diff: data.diff || '' }
  ]);
}

async function loadGitLog() {
  const limit = Math.max(1, Math.min(200, Number($('gitLogLimit').value) || 50));
  $('gitLogLimit').value = String(limit);
  const data = await api('/v1/git/log?limit=' + encodeURIComponent(limit));
  const list = $('gitLog');
  list.innerHTML = '';
  if (!data.git_repo) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = '当前 memory 目录不是 Git 仓库。';
    list.appendChild(empty);
    return;
  }
  if (!data.commits || !data.commits.length) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = '暂无提交历史。';
    list.appendChild(empty);
    return;
  }
  for (const commit of data.commits) {
    const item = document.createElement('div');
    item.className = 'commit';
    const title = document.createElement('div');
    title.className = 'commit-title';
    const subject = document.createElement('strong');
    subject.textContent = commit.subject || '(no subject)';
    const hash = document.createElement('span');
    hash.className = 'hash';
    hash.textContent = commit.short_hash;
    title.appendChild(subject);
    title.appendChild(hash);
    const meta = document.createElement('div');
    meta.className = 'meta';
    meta.textContent = [commit.author, commit.date].filter(Boolean).join(' · ');
    const full = document.createElement('div');
    full.className = 'tree-path';
    full.textContent = commit.hash;
    item.appendChild(title);
    item.appendChild(meta);
    item.appendChild(full);
    list.appendChild(item);
  }
}

async function loadSyncStatus() {
  const data = await api('/v1/sync/status');
  $('syncStatus').textContent = JSON.stringify(data, null, 2);
}

async function syncAction(action) {
  $('syncResult').textContent = '执行中…';
  const data = await api('/v1/sync/' + action, { method: 'POST' });
  $('syncResult').textContent = JSON.stringify(data, null, 2);
  await loadSyncStatus();
}

$('tabMemories').onclick = () => setTab('memories');
$('tabGit').onclick = () => setTab('git');
$('tabConfig').onclick = () => setTab('config');
$('refreshBtn').onclick = () => loadList().catch(e => toast(e.message, true));
$('searchBtn').onclick = () => doSearch().catch(e => toast(e.message, true));
$('searchInput').addEventListener('keydown', e => { if (e.key === 'Enter') doSearch().catch(err => toast(err.message, true)); });
$('newBtn').onclick = newMemory;
$('editBtn').onclick = () => setEditing(true);
$('cancelBtn').onclick = () => setEditing(false);
$('saveBtn').onclick = () => saveMemory().catch(e => toast(e.message, true));
$('deleteBtn').onclick = () => deleteMemory().catch(e => toast(e.message, true));
$('syncStatusBtn').onclick = () => loadSyncStatus().catch(e => toast(e.message, true));
$('gitDiffBtn').onclick = () => loadGitDiff().catch(e => toast(e.message, true));
$('gitLogBtn').onclick = () => loadGitLog().catch(e => toast(e.message, true));
$('pullBtn').onclick = () => syncAction('pull').catch(e => toast(e.message, true));
$('pushBtn').onclick = () => syncAction('push').catch(e => toast(e.message, true));
$('syncNowBtn').onclick = () => syncAction('now').catch(e => toast(e.message, true));

loadList().catch(e => toast(e.message, true));
`
