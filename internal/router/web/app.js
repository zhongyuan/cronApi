/* cronApi 控制台 —— 零依赖原生 JS */
(() => {
  "use strict";

  const API = "/api/v1";
  const $ = (sel) => document.querySelector(sel);
  const state = { token: sessionStorage.getItem("cronapi_token") || "", tasks: [], editingId: 0, steps: [], auths: [] };

  /* ---------------- 基础工具 ---------------- */
  async function api(path, opts = {}) {
    const headers = Object.assign({ "Content-Type": "application/json" }, opts.headers || {});
    if (state.token) headers.Authorization = "Bearer " + state.token;
    const res = await fetch(API + path, Object.assign({}, opts, { headers }));
    let body = null;
    try { body = await res.json(); } catch (_) { /* 非 JSON */ }
    if (!res.ok || (body && body.code !== 0)) {
      const msg = (body && body.msg) || ("HTTP " + res.status);
      const err = new Error(msg);
      err.code = body && body.code;
      throw err;
    }
    return body.data;
  }

  function toast(msg, isErr) {
    const el = $("#toast");
    el.textContent = msg;
    el.className = "toast" + (isErr ? " err" : "");
    el.hidden = false;
    clearTimeout(toast._t);
    toast._t = setTimeout(() => { el.hidden = true; }, 2600);
  }

  const esc = (s) => String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  function fmtTime(t) {
    if (!t) return "—";
    const d = new Date(t);
    if (isNaN(d)) return "—";
    const p = (n) => String(n).padStart(2, "0");
    return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate()) + " " + p(d.getHours()) + ":" + p(d.getMinutes());
  }

  function relTime(t) {
    if (!t) return "";
    const diff = new Date(t) - new Date();
    const abs = Math.abs(diff), m = Math.round(abs / 60000);
    const txt = m < 1 ? "不到 1 分钟" : m < 60 ? m + " 分钟" : m < 1440 ? Math.round(m / 60) + " 小时" : Math.round(m / 1440) + " 天";
    return diff >= 0 ? txt + "后" : txt + "前";
  }

  /* ---------------- cron 解析与描述 ---------------- */
  // 支持 5 段 / 6 段；返回 {ok, desc, next: Date[]}
  function parseCronField(expr, min, max) {
    // 返回 {any:bool, values:Set} 或 null
    const vals = new Set();
    let any = expr === "*" || expr === "?";
    const parts = String(expr).split(",");
    for (const part of parts) {
      if (!part) return null;
      let step = 1, range = part;
      const slash = part.split("/");
      if (slash.length === 2) {
        step = parseInt(slash[1], 10);
        if (!(step > 0)) return null;
        range = slash[0];
      } else if (slash.length > 2) return null;

      let lo, hi;
      if (range === "*" || range === "?") { lo = min; hi = max; }
      else if (range.includes("-")) {
        const [a, b] = range.split("-").map(Number);
        if (isNaN(a) || isNaN(b)) return null;
        lo = a; hi = b;
      } else {
        const n = Number(range);
        if (isNaN(n)) return null;
        lo = n; hi = slash.length === 2 ? max : n;
      }
      if (lo < min || hi > max || lo > hi) return null;
      for (let v = lo; v <= hi; v += step) vals.add(v);
      if (step !== 1) any = false;
      if (lo !== min || hi !== max) any = false;
    }
    return { any, values: [...vals].sort((a, b) => a - b) };
  }

  function describeCron(expr) {
    expr = (expr || "").trim();
    if (!expr) return { ok: false, desc: "请输入 cron 表达式" };
    if (expr.startsWith("@")) {
      const m = /^@every\s+(\d+)([smhd])$/.exec(expr);
      const map = { "@yearly": "每年 1 月 1 日 00:00", "@annually": "每年 1 月 1 日 00:00", "@monthly": "每月 1 日 00:00", "@weekly": "每周日 00:00", "@daily": "每天 00:00", "@midnight": "每天 00:00", "@hourly": "每小时整点" };
      if (map[expr]) return { ok: true, desc: map[expr], next: [] };
      if (m) { const unit = { s: "秒", m: "分钟", h: "小时", d: "天" }[m[2]]; return { ok: true, desc: "每隔 " + m[1] + " " + unit, next: [] }; }
      return { ok: false, desc: "无法识别的描述符" };
    }
    const f = expr.split(/\s+/);
    if (f.length !== 5 && f.length !== 6) return { ok: false, desc: "需要 5 段（分 时 日 月 周）或 6 段（秒 分 时 日 月 周）" };
    const hasSec = f.length === 6;
    const [secF, minF, hourF, domF, monF, dowF] = hasSec ? f : ["0", ...f];
    const sec = parseCronField(secF, 0, 59), min = parseCronField(minF, 0, 59), hour = parseCronField(hourF, 0, 23);
    const dom = parseCronField(domF, 1, 31), mon = parseCronField(monF, 1, 12), dow = parseCronField(dowF, 0, 7);
    if (!sec || !min || !hour || !dom || !mon || !dow) return { ok: false, desc: "表达式格式有误" };

    const parts = [];
    // 秒级
    if (hasSec && !sec.any) {
      if (sec.values.length === 1) parts.push("第 " + sec.values[0] + " 秒");
      else parts.push("第 " + sec.values.slice(0, 5).join("、") + (sec.values.length > 5 ? "…" : "") + " 秒");
    }
    // 时间粒度
    const everySecond = hasSec && sec.any;
    if (everySecond) parts.push("每秒");
    else if (min.any && hour.any) parts.push("每分钟");
    else if (hour.any && !min.any) {
      parts.push("每小时第 " + min.values.join("、") + " 分");
    } else if (!hour.any && min.any) {
      parts.push(hour.values.length === 1 ? "每天 " + pad(hour.values[0]) + ":" : hour.values.map(pad).join("、") + " 点");
    } else {
      const times = [];
      for (const h of hour.values) for (const m of min.values) times.push(pad(h) + ":" + pad(m));
      parts.push(times.length > 4 ? times.slice(0, 4).join("、") + " 等 " + times.length + " 个时间" : times.join("、"));
    }
    // 日期约束
    if (!dom.any) parts.push("每月 " + dom.values.join("、") + " 日");
    if (!mon.any) parts.push(mon.values.join("、") + " 月");
    if (!dow.any) parts.push("周" + dow.values.map((d) => "日一二三四五六"[d % 7]).join("、"));

    const next = nextRuns({ sec, min, hour, dom, mon, dow, hasSec }, 3);
    return { ok: true, desc: parts.join(" · "), next };
  }

  const pad = (n) => String(n).padStart(2, "0");

  function nextRuns(f, count) {
    const out = [];
    // 从「下一整单位」开始搜索：秒级从下一秒，分钟级从下一分钟
    const t = new Date();
    if (f.hasSec) {
      t.setSeconds(t.getSeconds() + 1, 0);
    } else {
      t.setSeconds(0, 0);
      t.setMinutes(t.getMinutes() + 1);
    }
    const limit = new Date(t.getTime() + 366 * 24 * 3600 * 1000);
    const domRestricted = !f.dom.any, dowRestricted = !f.dow.any;
    while (t < limit && out.length < count) {
      if (!f.mon.values.includes(t.getMonth() + 1) && !f.mon.any) { t.setDate(1); t.setHours(0, 0, 0, 0); continue; }
      const dowOk = f.dow.any || f.dow.values.includes(t.getDay());
      const domOk = f.dom.any || f.dom.values.includes(t.getDate());
      // cron 标准：日与周同时受限时取并集
      const dayOk = domRestricted && dowRestricted ? (domOk || dowOk) : (domOk && dowOk);
      if (!dayOk) { t.setDate(t.getDate() + 1); t.setHours(0, 0, 0, 0); continue; }
      if (!f.hour.any && !f.hour.values.includes(t.getHours())) { t.setHours(t.getHours() + 1, 0, 0, 0); continue; }
      if (!f.min.any && !f.min.values.includes(t.getMinutes())) { t.setMinutes(t.getMinutes() + 1, 0, 0); continue; }
      if (f.hasSec && !f.sec.any && !f.sec.values.includes(t.getSeconds())) { t.setSeconds(t.getSeconds() + 1, 0); continue; }
      out.push(new Date(t));
      t.setSeconds(t.getSeconds() + (f.hasSec ? 1 : 60), 0);
    }
    return out;
  }

  const PRESETS = [
    { label: "每分钟", expr: "* * * * *" },
    { label: "每 5 分钟", expr: "*/5 * * * *" },
    { label: "每小时", expr: "0 * * * *" },
    { label: "每天 9 点", expr: "0 9 * * *" },
    { label: "工作日 9 点", expr: "0 9 * * 1-5" },
    { label: "每 30 秒", expr: "*/30 * * * * *" },
  ];

  /* ---------------- 登录 ---------------- */
  async function ensureAuth() {
    if (state.token) {
      try {
        const me = await api("/auth/me");
        $("#whoami").textContent = me.nickname || me.open_id;
        showMain();
        return;
      } catch (_) { state.token = ""; sessionStorage.removeItem("cronapi_token"); }
    }
    $("#loginView").hidden = false;
    $("#mainView").hidden = true;
  }

  async function doLogin() {
    $("#loginErr").textContent = "";
    $("#loginBtn").disabled = true;
    try {
      const data = await api("/auth/wechat/login", { method: "POST", body: JSON.stringify({ code: $("#loginCode").value.trim() }) });
      state.token = data.access_token;
      sessionStorage.setItem("cronapi_token", state.token);
      await ensureAuth();
    } catch (e) {
      $("#loginErr").textContent = e.message;
    } finally {
      $("#loginBtn").disabled = false;
    }
  }

  function showMain() {
    $("#loginView").hidden = true;
    $("#mainView").hidden = false;
    loadTasks();
  }

  /* ---------------- 任务列表 ---------------- */
  async function loadTasks() {
    try {
      const data = await api("/tasks?page=1&page_size=100");
      state.tasks = data.list || [];
      renderStats();
      renderTasks();
    } catch (e) {
      toast(e.message, true);
      if (e.code === 10001 || e.code === 10002 || e.code === 10004) { state.token = ""; sessionStorage.removeItem("cronapi_token"); ensureAuth(); }
    }
  }

  function renderStats() {
    const ts = state.tasks;
    const on = ts.filter((t) => t.enabled).length;
    const ran = ts.filter((t) => t.last_run_at).length;
    const soon = ts.filter((t) => t.enabled).map((t) => describeCron(t.cron_expr).next[0]).filter(Boolean).sort((a, b) => a - b)[0];
    $("#stats").innerHTML =
      stat("任务总数", ts.length) +
      stat("已启用", on) +
      stat("已执行过", ran) +
      stat("下次执行", soon ? relTime(soon.toISOString()) : "—");
  }
  const stat = (k, v) => '<div class="stat"><div class="k">' + k + '</div><div class="v">' + esc(v) + "</div></div>";

  function renderTasks() {
    const wrap = $("#taskList");
    if (!state.tasks.length) {
      wrap.innerHTML = '<div class="empty"><div class="big">⏱</div><p>还没有任务</p><p class="muted" style="margin-top:6px">点击右上角「新建任务」，添加第一个接口与定时规则</p></div>';
      return;
    }
    wrap.innerHTML = state.tasks.map((t) => {
      const c = describeCron(t.cron_expr);
      const next = t.enabled && c.next[0] ? "下次执行 " + relTime(c.next[0].toISOString()) : t.enabled ? "" : "已停用，不会自动执行";
      return (
        '<div class="task' + (t.enabled ? "" : " off") + '" data-id="' + t.id + '">' +
          '<div class="task-head">' +
            '<span class="dot ' + (t.enabled ? "on" : "off") + '"></span>' +
            '<span class="task-name">' + esc(t.name) + "</span>" +
            '<span class="tag cron">' + esc(t.cron_expr) + "</span>" +
            (c.ok ? '<span class="tag">' + esc(c.desc) + "</span>" : "") +
          "</div>" +
          '<div class="task-meta">' +
            "<span>" + esc(t.base_url || "") + "</span>" +
            "<span>" + (t.steps || []).length + " 个步骤</span>" +
            "<span>" + (next ? esc(next) : "") + "</span>" +
            "<span>上次执行 " + (t.last_run_at ? esc(fmtTime(t.last_run_at)) : "从未") + "</span>" +
          "</div>" +
          '<div class="task-actions">' +
            '<button class="btn btn-ghost btn-sm" data-act="runs" data-id="' + t.id + '">执行记录</button>' +
            '<button class="btn" data-act="run" data-id="' + t.id + '">立即执行</button>' +
            '<button class="btn" data-act="edit" data-id="' + t.id + '">编辑</button>' +
            '<label class="switch-field" title="启用/停用"><input type="checkbox" class="switch" data-act="toggle" data-id="' + t.id + '"' + (t.enabled ? " checked" : "") + " /></label>" +
            '<button class="btn btn-ghost btn-sm" data-act="del" data-id="' + t.id + '" style="color:var(--err)">删除</button>' +
          "</div>" +
        "</div>"
      );
    }).join("");
  }

  /* ---------------- 新建 / 编辑 ---------------- */
  function openDrawer(task) {
    state.editingId = task ? task.id : 0;
    state.steps = task && task.steps && task.steps.length ? JSON.parse(JSON.stringify(task.steps)) : [blankStep()];
    state.auths = task && task.auths ? Object.entries(task.auths).map(([name, cfg]) => Object.assign({ name }, cfg)) : [];
    $("#drawerTitle").textContent = task ? "编辑任务" : "新建任务";
    $("#fName").value = task ? task.name : "";
    $("#fDesc").value = task ? task.description || "" : "";
    $("#fBaseURL").value = task ? task.base_url || "" : "http://127.0.0.1:8080";
    $("#fCron").value = task ? task.cron_expr : "*/5 * * * *";
    $("#fRetry").value = task ? task.max_retry : 1;
    $("#fTimeout").value = task ? task.timeout_sec : 60;
    $("#fEnabled").checked = task ? !!task.enabled : true;
    $("#formErr").textContent = "";
    renderPresets();
    renderCronPreview();
    renderSteps();
    renderAuths();
    $("#drawer").hidden = false;
    $("#drawerMask").hidden = false;
  }

  function closeDrawer() {
    $("#drawer").hidden = true;
    $("#drawerMask").hidden = true;
  }

  function blankStep() {
    return { name: "", method: "GET", url: "", auth: "inherit", headers: {}, body: "", asserts: [], extracts: {} };
  }

  function renderPresets() {
    const cur = $("#fCron").value.trim();
    $("#presetRow").innerHTML = PRESETS.map((p) =>
      '<button class="chip' + (p.expr === cur ? " active" : "") + '" data-expr="' + esc(p.expr) + '">' + esc(p.label) + "</button>"
    ).join("");
  }

  function renderCronPreview() {
    const el = $("#cronPreview");
    const r = describeCron($("#fCron").value);
    if (!r.ok) { el.className = "cron-preview bad"; el.textContent = "⚠ " + r.desc; return; }
    el.className = "cron-preview";
    const nexts = r.next.slice(0, 3).map((d) => fmtTime(d.toISOString())).join("　");
    el.innerHTML = "<code>" + esc(r.desc) + "</code>" + (nexts ? "<br/>接下来执行：" + esc(nexts) : "");
  }

  function renderSteps() {
    // 重绘会让输入框失焦，重建前记录焦点与滚动，重建后恢复
    const active = document.activeElement;
    const focusKey = active && active.dataset ? {
      k: active.dataset.k, step: active.dataset.step, auth: active.dataset.authk,
      ak: active.dataset.ak, ai: active.dataset.ai, ek: active.dataset.ek, hk: active.dataset.hk, ri: active.dataset.ri,
      selStart: active.selectionStart, selEnd: active.selectionEnd,
    } : null;
    const body = $("#drawer") && document.querySelector(".drawer-body");
    const scrollTop = body ? body.scrollTop : 0;

    $("#stepCount").textContent = "（" + state.steps.length + "）";
    $("#stepsWrap").innerHTML = state.steps.map((s, i) => stepCard(s, i)).join("");

    if (focusKey && focusKey.step !== undefined && focusKey.step !== null) {
      let sel = "";
      if (focusKey.k) sel = '[data-k="' + focusKey.k + '"][data-step="' + focusKey.step + '"]';
      else if (focusKey.auth !== undefined) sel = '[data-authk="' + focusKey.auth + '"][data-authi="' + focusKey.step + '"]';
      else if (focusKey.ak) sel = '[data-ak="' + focusKey.ak + '"][data-step="' + focusKey.step + '"][data-ai="' + focusKey.ai + '"]';
      else if (focusKey.ek) sel = '[data-ek="' + focusKey.ek + '"][data-step="' + focusKey.step + '"][data-ri="' + focusKey.ri + '"]';
      else if (focusKey.hk) sel = '[data-hk="' + focusKey.hk + '"][data-step="' + focusKey.step + '"][data-ri="' + focusKey.ri + '"]';
      const el = sel ? document.querySelector(sel) : null;
      if (el) {
        el.focus({ preventScroll: true });
        try {
          if (focusKey.selStart != null && el.setSelectionRange) el.setSelectionRange(focusKey.selStart, focusKey.selEnd);
        } catch (_) { /* 非文本输入不支持 */ }
      }
    }
    const body2 = document.querySelector(".drawer-body");
    if (body2) body2.scrollTop = scrollTop;
  }

  function stepCard(s, i) {
    const authOpts = [["inherit", "沿用调用者登录态"], ["none", "不带鉴权"], ["login", "自动登录"]]
      .map(([v, l]) => '<option value="' + v + '"' + (s.auth === v ? " selected" : "") + ">" + l + "</option>").join("");
    const asserts = (s.asserts || []).map((a, ai) => assertRow(a, i, ai)).join("");
    const extracts = Object.entries(s.extracts || {}).map(([k, v], ri) => extractRow(k, v, i, ri)).join("");
    const headers = Object.entries(s.headers || {}).map(([k, v], ri) => headerRow(k, v, i, ri)).join("");
    return (
      '<div class="step-card" data-step="' + i + '">' +
        '<div class="step-head">' +
          '<span class="step-idx">' + (i + 1) + "</span>" +
          '<input data-k="name" data-step="' + i + '" placeholder="步骤名称，如：查询订单列表" value="' + esc(s.name) + '" />' +
          '<button class="row-del" data-act="delStep" data-step="' + i + '" title="删除步骤">✕</button>' +
        "</div>" +
        '<div class="step-grid">' +
          '<select data-k="method" data-step="' + i + '">' +
            ["GET", "POST", "PUT", "PATCH", "DELETE"].map((m) => '<option' + (s.method === m ? " selected" : "") + ">" + m + "</option>").join("") +
          "</select>" +
          '<input data-k="url" data-step="' + i + '" placeholder="/api/v1/orders 或完整 URL" value="' + esc(s.url) + '" />' +
        "</div>" +
        '<div class="step-grid">' +
          '<select data-k="auth" data-step="' + i + '">' + authOpts + "</select>" +
          (s.auth === "login"
            ? '<select data-k="auth_ref" data-step="' + i + '">' + authOptions(s.auth_ref) + "</select>"
            : '<input data-k="auth_ref" data-step="' + i + '" placeholder="（可选）请求头/路径参数，可写 {{变量}}" disabled />') +
        "</div>" +
        '<textarea data-k="body" data-step="' + i + '" placeholder="请求体 JSON（可选），支持 {{变量}}" rows="2">' + esc(s.body) + "</textarea>" +
        '<div class="section-title" style="margin-top:10px;font-size:12.5px">断言 <button class="btn btn-mini" data-act="addAssert" data-step="' + i + '">+ 断言</button></div>' +
        asserts +
        '<div class="section-title" style="margin-top:10px;font-size:12.5px">变量提取（响应路径→变量） <button class="btn btn-mini" data-act="addExtract" data-step="' + i + '">+ 提取</button></div>' +
        extracts +
        '<div class="section-title" style="margin-top:10px;font-size:12.5px">请求头 <button class="btn btn-mini" data-act="addHeader" data-step="' + i + '">+ 请求头</button></div>' +
        headers +
      "</div>"
    );
  }

  function authOptions(sel) {
    if (!state.auths.length) return '<option value="">（请先添加登录配置）</option>';
    const exists = state.auths.some((a) => a.name === sel);
    const current = exists ? sel : state.auths[0].name;
    return state.auths.map((a) => '<option value="' + esc(a.name) + '"' + (a.name === current ? " selected" : "") + ">" + esc(a.name) + "</option>").join("");
  }

  function assertRow(a, i, ai) {
    const ops = ["eq", "ne", "contains", "not_contains", "exists", "not_exists", "gt", "lt", "status_in"];
    return (
      '<div class="step-grid" style="grid-template-columns:1fr 110px 1fr 28px" data-assert="' + ai + '">' +
        '<input data-ak="source" data-step="' + i + '" data-ai="' + ai + '" placeholder="status 或 json:data.id" value="' + esc(a.source || "status") + '" />' +
        '<select data-ak="op" data-step="' + i + '" data-ai="' + ai + '">' + ops.map((o) => '<option' + (a.op === o ? " selected" : "") + ">" + o + "</option>").join("") + "</select>" +
        '<input data-ak="value" data-step="' + i + '" data-ai="' + ai + '" placeholder="期望值" value="' + esc(a.value || "") + '" />' +
        '<button class="row-del" data-act="delAssert" data-step="' + i + '" data-ai="' + ai + '">✕</button>' +
      "</div>"
    );
  }
  function extractRow(k, v, i, ri) {
    return (
      '<div class="step-grid" style="grid-template-columns:1fr 1fr 28px">' +
        '<input data-ek="name" data-step="' + i + '" data-ri="' + ri + '" placeholder="变量名" value="' + esc(k) + '" />' +
        '<input data-ek="path" data-step="' + i + '" data-ri="' + ri + '" placeholder="data.order_id" value="' + esc(v) + '" />' +
        '<button class="row-del" data-act="delExtract" data-step="' + i + '" data-ri="' + ri + '">✕</button>' +
      "</div>"
    );
  }
  function headerRow(k, v, i, ri) {
    return (
      '<div class="step-grid" style="grid-template-columns:1fr 1fr 28px">' +
        '<input data-hk="name" data-step="' + i + '" data-ri="' + ri + '" placeholder="Header 名" value="' + esc(k) + '" />' +
        '<input data-hk="value" data-step="' + i + '" data-ri="' + ri + '" placeholder="值" value="' + esc(v) + '" />' +
        '<button class="row-del" data-act="delHeader" data-step="' + i + '" data-ri="' + ri + '">✕</button>' +
      "</div>"
    );
  }

  function renderAuths() {
    $("#authsWrap").innerHTML = state.auths.length
      ? state.auths.map((a, i) =>
          '<div class="step-card">' +
            '<div class="step-grid">' +
              '<input data-authk="name" data-authi="' + i + '" placeholder="配置名，如 wx" value="' + esc(a.name) + '" />' +
              '<input data-authk="code" data-authi="' + i + '" placeholder="登录码，mock 模式填 openid:u1001" value="' + esc(a.code || "") + '" />' +
            "</div>" +
            '<div class="step-grid">' +
              '<select data-authk="type" data-authi="' + i + '">' +
                ['wechat_miniprogram', 'custom'].map((t) => '<option' + (a.type === t ? " selected" : "") + ">" + t + "</option>").join("") +
              "</select>" +
              '<input data-authk="url" data-authi="' + i + '" placeholder="custom 时填登录接口地址" value="' + esc(a.url || "") + '" />' +
            "</div>" +
          "</div>"
        ).join("")
      : '<p class="hint">暂无，步骤不选「自动登录」可忽略。</p>';
  }

  /* ---------------- 表单收集与保存 ---------------- */
  function collect() {
    const steps = state.steps.map((s, i) => {
      const st = { name: s.name || "步骤 " + (i + 1), method: s.method, url: s.url, auth: s.auth };
      if (s.auth === "login" && s.auth_ref) st.auth_ref = s.auth_ref;
      if (s.body) st.body = s.body;
      if (Object.keys(s.headers || {}).length) st.headers = s.headers;
      if (Object.keys(s.extracts || {}).length) st.extracts = s.extracts;
      if ((s.asserts || []).length) st.asserts = s.asserts;
      return st;
    });
    const auths = {};
    state.auths.forEach((a) => { if (a.name) auths[a.name] = { type: a.type || "wechat_miniprogram", code: a.code || "", url: a.url || "" }; });
    return {
      name: $("#fName").value.trim(),
      description: $("#fDesc").value.trim(),
      base_url: $("#fBaseURL").value.trim(),
      cron_expr: $("#fCron").value.trim(),
      timezone: "Asia/Shanghai",
      enabled: $("#fEnabled").checked,
      max_retry: Number($("#fRetry").value) || 0,
      timeout_sec: Number($("#fTimeout").value) || 60,
      steps,
      auths,
    };
  }

  function validate(p) {
    if (!p.name) return "请填写任务名称";
    if (!/^https?:\/\//.test(p.base_url)) return "被测服务地址需以 http:// 或 https:// 开头";
    const c = describeCron(p.cron_expr);
    if (!c.ok) return "定时规则有误：" + c.desc;
    if (!p.steps.length) return "至少需要一个接口步骤";
    for (let i = 0; i < p.steps.length; i++) {
      if (!p.steps[i].url) return "第 " + (i + 1) + " 个步骤缺少接口路径";
      if (p.steps[i].auth === "login" && !p.steps[i].auth_ref) return "第 " + (i + 1) + " 个步骤选择了自动登录但未指定登录配置";
    }
    return "";
  }

  async function save() {
    const payload = collect();
    const err = validate(payload);
    if (err) { $("#formErr").textContent = err; return; }
    $("#saveBtn").disabled = true;
    try {
      if (state.editingId) {
        await api("/tasks/" + state.editingId, { method: "PUT", body: JSON.stringify(payload) });
        toast("已保存，定时规则即时生效");
      } else {
        await api("/tasks", { method: "POST", body: JSON.stringify(payload) });
        toast("任务已创建");
      }
      closeDrawer();
      loadTasks();
    } catch (e) {
      $("#formErr").textContent = e.message;
    } finally {
      $("#saveBtn").disabled = false;
    }
  }

  /* ---------------- 执行与报告 ---------------- */
  async function runTask(id) {
    const btn = document.querySelector('[data-act="run"][data-id="' + id + '"]');
    if (btn) { btn.disabled = true; btn.textContent = "执行中…"; }
    try {
      const run = await api("/tasks/" + id + "/run", { method: "POST" });
      showRun(run);
      loadTasks();
    } catch (e) {
      toast(e.message, true);
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = "立即执行"; }
    }
  }

  function showRun(run) {
    const ok = run.status === "success";
    const steps = (run.steps || []).map((s) => {
      const cls = s.skipped ? "skip" : s.passed ? "" : "fail";
      const icon = s.skipped ? "○" : s.passed ? "✓" : "✕";
      return (
        '<div class="rs ' + cls + '">' +
          '<div class="rs-head"><b style="color:inherit">' + icon + "</b><strong>" + esc(s.name) + "</strong>" +
            '<span class="code">' + esc(s.method || "") + " " + esc(s.url || "") + "</span>" +
            '<span class="tag" style="margin-left:auto">' + (s.status || "-") + " · " + (s.duration_ms || 0) + "ms" +
            (s.attempts > 1 ? " · 重试 " + (s.attempts - 1) : "") + "</span></div>" +
          (s.failures && s.failures.length ? '<div class="rs-fail">' + s.failures.map(esc).join("<br/>") + "</div>" : "") +
          (s.error ? '<div class="rs-fail">' + esc(s.error) + "</div>" : "") +
          (s.response ? '<pre class="raw">' + esc(JSON.stringify(s.response, null, 2)) + "</pre>" : "") +
        "</div>"
      );
    }).join("");
    $("#runBody").innerHTML =
      '<div class="stats" style="grid-template-columns:1fr 1fr 1fr;margin-bottom:14px">' +
        stat("结果", ok ? "通过" : "失败") +
        stat("步骤", run.passed_steps + " / " + run.total_steps) +
        stat("耗时", run.duration_ms + " ms") +
      "</div>" +
      (run.error_msg ? '<p class="rs-fail" style="margin-bottom:10px">' + esc(run.error_msg) + "</p>" : "") +
      '<div class="report-steps">' + steps + "</div>";
    $("#runPanel").hidden = false;
    $("#runMask").hidden = false;
  }

  async function showRuns(taskId) {
    try {
      const data = await api("/tasks/" + taskId + "/runs?page=1&page_size=20");
      const list = data.list || [];
      const rows = list.map((r) => {
        const cls = r.status === "success" ? "s-ok" : r.status === "running" ? "s-run" : "s-fail";
        return '<div class="rs" style="cursor:pointer" data-run="' + r.id + '"><div class="rs-head">' +
          '<span class="status-pill ' + cls + '">' + esc(r.status) + "</span>" +
          "<span>" + esc(fmtTime(r.started_at)) + "</span>" +
          '<span class="tag">' + esc(r.trigger) + "</span>" +
          '<span class="tag" style="margin-left:auto">' + r.passed_steps + "/" + r.total_steps + " · " + r.duration_ms + "ms</span>" +
        "</div></div>";
      }).join("");
      $("#runBody").innerHTML = list.length ? '<p class="hint">最近 ' + list.length + " 次执行，点击查看详情</p>" + '<div class="report-steps">' + rows + "</div>" : '<div class="empty">暂无执行记录</div>';
      $("#runPanel").hidden = false;
      $("#runMask").hidden = false;
    } catch (e) { toast(e.message, true); }
  }

  /* ---------------- 事件绑定 ---------------- */
  function bind() {
    $("#loginBtn").onclick = doLogin;
    $("#loginCode").onkeydown = (e) => { if (e.key === "Enter") doLogin(); };
    $("#logoutBtn").onclick = () => { state.token = ""; sessionStorage.removeItem("cronapi_token"); ensureAuth(); };
    $("#newTaskBtn").onclick = () => openDrawer(null);
    $("#refreshBtn").onclick = loadTasks;
    $("#closeDrawer").onclick = closeDrawer;
    $("#cancelBtn").onclick = closeDrawer;
    $("#drawerMask").onclick = closeDrawer;
    $("#saveBtn").onclick = save;
    $("#closeRun").onclick = () => { $("#runPanel").hidden = true; $("#runMask").hidden = true; };
    $("#runMask").onclick = () => { $("#runPanel").hidden = true; $("#runMask").hidden = true; };
    $("#addStepBtn").onclick = () => { state.steps.push(blankStep()); renderSteps(); };
    $("#addAuthBtn").onclick = () => {
      const names = new Set(state.auths.map((a) => a.name));
      let name = "wx", n = 2;
      while (names.has(name)) name = "wx" + n++;
      state.auths.push({ name, type: "wechat_miniprogram", code: "openid:u1001" });
      // 步骤里选了自动登录但还没绑定时，自动指向新配置，避免保存时报错
      state.steps.forEach((s) => { if (s.auth === "login" && !s.auth_ref) s.auth_ref = name; });
      renderAuths();
      renderSteps();
    };
    $("#fCron").oninput = () => { renderCronPreview(); renderPresets(); };

    $("#presetRow").onclick = (e) => {
      const b = e.target.closest("[data-expr]");
      if (!b) return;
      $("#fCron").value = b.dataset.expr;
      renderCronPreview(); renderPresets();
    };

    // 列表操作
    $("#taskList").onclick = async (e) => {
      const el = e.target.closest("[data-act]");
      if (!el || el.dataset.act === "toggle") return;
      const id = Number(el.dataset.id);
      const task = state.tasks.find((t) => t.id === id);
      switch (el.dataset.act) {
        case "edit": openDrawer(task); break;
        case "run": runTask(id); break;
        case "runs": showRuns(id); break;
        case "del":
          if (!confirm("确认删除任务「" + task.name + "」？执行记录将保留。")) return;
          try { await api("/tasks/" + id, { method: "DELETE" }); toast("已删除"); loadTasks(); }
          catch (err) { toast(err.message, true); }
          break;
      }
    };
    $("#taskList").onchange = async (e) => {
      const el = e.target.closest('[data-act="toggle"]');
      if (!el) return;
      try {
        await api("/tasks/" + el.dataset.id + "/enabled", { method: "PATCH", body: JSON.stringify({ enabled: el.checked }) });
        toast(el.checked ? "已启用定时执行" : "已停用");
        loadTasks();
      } catch (err) { toast(err.message, true); el.checked = !el.checked; }
    };

    // 表单输入（步骤 / 断言 / 提取 / 登录配置）
    const body = $("#formBody");
    body.addEventListener("input", onFormInput);
    body.addEventListener("change", onFormInput);
    body.addEventListener("click", onFormClick);
  }

  function onFormInput(e) {
    const t = e.target;
    const i = Number(t.dataset.step);
    if (t.dataset.authk) {
      const ai = Number(t.dataset.authi);
      state.auths[ai][t.dataset.authk] = t.value;
      if (t.dataset.authk === "name") renderSteps();
      return;
    }
    if (!isNaN(i) && t.dataset.k) {
      state.steps[i][t.dataset.k] = t.value;
      if (t.dataset.k === "auth") {
        if (t.value === "login" && !state.steps[i].auth_ref && state.auths.length) {
          state.steps[i].auth_ref = state.auths[0].name;
        }
        renderSteps();
      }
      return;
    }
    if (!isNaN(i) && t.dataset.ak) {
      const ai = Number(t.dataset.ai);
      state.steps[i].asserts[ai][t.dataset.ak] = t.value;
      return;
    }
    if (!isNaN(i) && t.dataset.ek) { setMapEntry(i, "extracts", t); return; }
    if (!isNaN(i) && t.dataset.hk) { setMapEntry(i, "headers", t); return; }
  }

  // 按下标把输入写回对象，键名改变时保持行顺序不变
  function setMapEntry(i, field, input) {
    const row = input.closest(".step-grid");
    if (!row) return;
    const nameSel = field === "extracts" ? '[data-ek="name"]' : '[data-hk="name"]';
    const valSel = field === "extracts" ? '[data-ek="path"]' : '[data-hk="value"]';
    const entries = Object.entries(state.steps[i][field] || {});
    entries[Number(input.dataset.ri)] = [row.querySelector(nameSel).value.trim(), row.querySelector(valSel).value];
    state.steps[i][field] = Object.fromEntries(entries.filter((e) => e[0]));
  }

  function deleteMapEntry(i, field, ri) {
    const entries = Object.entries(state.steps[i][field] || {});
    entries.splice(ri, 1);
    state.steps[i][field] = Object.fromEntries(entries);
    renderSteps();
  }

  function onFormClick(e) {
    const b = e.target.closest("[data-act]");
    if (!b) return;
    const i = Number(b.dataset.step);
    switch (b.dataset.act) {
      case "delStep": state.steps.splice(i, 1); if (!state.steps.length) state.steps.push(blankStep()); renderSteps(); break;
      case "addAssert": state.steps[i].asserts = state.steps[i].asserts || []; state.steps[i].asserts.push({ source: "status", op: "eq", value: "200" }); renderSteps(); break;
      case "delAssert": state.steps[i].asserts.splice(Number(b.dataset.ai), 1); renderSteps(); break;
      case "addExtract": {
        const ex = state.steps[i].extracts || (state.steps[i].extracts = {});
        let n = Object.keys(ex).length + 1;
        while (ex["var" + n]) n++;
        ex["var" + n] = "";
        renderSteps();
        break;
      }
      case "delExtract": deleteMapEntry(i, "extracts", Number(b.dataset.ri)); break;
      case "addHeader": state.steps[i].headers = state.steps[i].headers || {}; state.steps[i].headers["X-Header " + (Object.keys(state.steps[i].headers).length + 1)] = ""; renderSteps(); break;
      case "delHeader": deleteMapEntry(i, "headers", Number(b.dataset.ri)); break;
    }
  }

  /* ---------------- 启动 ---------------- */
  bind();
  ensureAuth();
})();
