const segmentOrder = ["早上", "中午", "下午", "晚上"];
const apiBases = ["/_xueya", "/api"];
let activeApiBase = apiBases[0];
let authToken = "";
const appVersion = "0.1.5";
const requestTimeoutMs = 6000;

function api(url, opts = {}, base = activeApiBase) {
  url = apiURL(url, base);
  opts.credentials = "same-origin";
  const token = getAuthToken();
  if (token) {
    opts.headers = { ...opts.headers, Authorization: "Bearer " + token };
  }
  return fetchWithTimeout(url, opts);
}

function apiURL(url, base = activeApiBase, includeToken = true) {
  if (/^https?:\/\//i.test(url)) return url;
  const path = url.startsWith("/api/") ? url.slice(4) : url;
  const full = `${base}${path.startsWith("/") ? path : `/${path}`}`;
  const token = includeToken ? getAuthToken() : "";
  if (!token) return full;
  const separator = full.includes("?") ? "&" : "?";
  return `${full}${separator}access_token=${encodeURIComponent(token)}`;
}

function fetchWithTimeout(url, opts = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), requestTimeoutMs);
  return fetch(url, { ...opts, signal: controller.signal })
    .finally(() => clearTimeout(timer));
}

async function readJSON(response) {
  const text = await response.text();
  try {
    return text ? JSON.parse(text) : {};
  } catch {
    const sample = text.replace(/\s+/g, " ").slice(0, 120);
    throw new Error(`接口没有返回 JSON：HTTP ${response.status} ${sample}`);
  }
}

async function apiJSON(url, opts = {}) {
  let lastError;
  const bases = [activeApiBase, ...apiBases.filter((base) => base !== activeApiBase)];
  for (const base of bases) {
    try {
      const response = await api(url, opts, base);
      const data = await readJSON(response);
      activeApiBase = base;
      return { response, data };
    } catch (error) {
      lastError = error;
      console.warn("api fallback:", base, error);
    }
  }
  throw lastError || new Error("无法连接后端接口");
}

function getAuthToken() {
  if (location.hash.startsWith("#token=")) {
    const newToken = decodeURIComponent(location.hash.slice("#token=".length));
    if (newToken) {
      authToken = newToken;
      rememberToken(authToken);
      history.replaceState(null, "", location.pathname + location.search);
      return authToken;
    }
  }
  if (authToken) return authToken;
  try {
    authToken = localStorage.getItem("token") || "";
  } catch { /* localStorage unavailable */ }
  return authToken;
}

function rememberToken(token) {
  authToken = token || "";
  if (!authToken) return;
  try { localStorage.setItem("token", authToken); } catch { /* localStorage unavailable */ }
}

function forgetToken() {
  authToken = "";
  try { localStorage.removeItem("token"); } catch { /* localStorage unavailable */ }
  if (location.hash.startsWith("#token=")) {
    history.replaceState(null, "", location.pathname + location.search);
  }
}

const state = {
  records: [],
  issues: [],
  month: "",
  date: "",
  segment: "all",
};

const els = {
  monthSelect: document.querySelector("#monthSelect"),
  dateSearch: document.querySelector("#dateSearch"),
  summary: document.querySelector("#summary"),
  dayTable: document.querySelector("#dayTable"),
  issues: document.querySelector("#issues"),
  rangeLabel: document.querySelector("#rangeLabel"),
  glucoseChart: document.querySelector("#glucoseChart"),
  pressureChart: document.querySelector("#pressureChart"),
  form: document.querySelector("#recordForm"),
  message: document.querySelector("#message"),
  exportCsv: document.querySelector("#exportCsv"),
  exportXlsx: document.querySelector("#exportXlsx"),
  importXlsx: document.querySelector("#importXlsx"),
  cameraInput: document.querySelector("#cameraInput"),
  cameraLabel: document.querySelector(".camera-button"),
  loginOverlay: document.querySelector("#loginOverlay"),
  loginForm: document.querySelector("#loginForm"),
  loginTitle: document.querySelector("#loginTitle"),
  loginHint: document.querySelector("#loginHint"),
  loginUsername: document.querySelector("#loginUsername"),
  loginPassword: document.querySelector("#loginPassword"),
  loginSubmit: document.querySelector("#loginSubmit"),
  loginError: document.querySelector("#loginError"),
  buildVersion: document.querySelector("#buildVersion"),
  mainApp: document.querySelector("#mainApp"),
  settingsBtn: document.querySelector("#settingsBtn"),
  settingsOverlay: document.querySelector("#settingsOverlay"),
  settingsDataPath: document.querySelector("#settingsDataPath"),
  settingsVisionURL: document.querySelector("#settingsVisionURL"),
  settingsVisionKey: document.querySelector("#settingsVisionKey"),
  settingsVisionModel: document.querySelector("#settingsVisionModel"),
  settingsOldPassword: document.querySelector("#settingsOldPassword"),
  settingsNewPassword: document.querySelector("#settingsNewPassword"),
  saveSettings: document.querySelector("#saveSettings"),
  settingsMessage: document.querySelector("#settingsMessage"),
  closeSettings: document.querySelector("#closeSettings"),
  logoutButton: document.querySelector("#logoutButton"),
};

init();

async function init() {
  bindAuthEvents();
  getAuthToken();
  await checkAuth();
}

let authBusy = false;
let authEventsBound = false;

function bindAuthEvents() {
  if (authEventsBound) return;
  authEventsBound = true;
  els.loginForm.addEventListener("submit", login);
}

async function checkAuth() {
  if (authBusy) return;
  authBusy = true;
  try {
    const { data } = await apiJSON("/check");
    setBuildVersion(data);
    if (data.authed) {
      await showApp();
      authBusy = false;
      return;
    }
    showLogin(data);
  } catch(e) {
    showLogin({
      has_password: true,
      version: appVersion,
      warning: "无法连接后端，请检查服务状态。",
      error: e?.message || "无法连接后端接口",
    });
  }
  authBusy = false;
}

function showLogin(data) {
  els.mainApp.style.display = "none";
  setBuildVersion(data);
  const needsSetup = !data?.has_password;
  els.loginTitle.textContent = needsSetup ? "设置访问密码" : "登录";
  els.loginHint.textContent = data?.warning || (needsSetup ? "首次使用请设置访问凭据" : "请输入凭据以继续");
  els.loginOverlay.style.display = "flex";
}

function setBuildVersion(data) {
  if (!els.buildVersion) return;
  const version = data?.version || appVersion;
  const commit = data?.commit && data.commit !== "dev" ? ` ${data.commit.slice(0, 7)}` : "";
  els.buildVersion.textContent = `v${version}${commit}`;
}

function toLogin() {
  els.mainApp.style.display = "none";
  els.settingsOverlay.style.display = "none";
  checkAuth();
}

async function showApp() {
  els.loginOverlay.style.display = "none";
  els.mainApp.style.display = "block";
  setTimeout(() => els.mainApp.classList.add("ready"), 50);
  setDefaultFormDate();
  bindEvents();
  await loadConfig();
  await loadRecords();
}

let eventsBound = false;

function bindEvents() {
  if (eventsBound) return;
  eventsBound = true;

  els.monthSelect.addEventListener("change", () => {
    state.month = els.monthSelect.value;
    render();
  });

  els.dateSearch.addEventListener("change", () => {
    state.date = els.dateSearch.value;
    render();
  });

  document.querySelectorAll(".segment-btn").forEach((button) => {
    button.addEventListener("click", () => {
      document.querySelectorAll(".segment-btn").forEach((item) => item.classList.remove("active"));
      button.classList.add("active");
      state.segment = button.dataset.segment;
      render();
    });
  });

  els.form.addEventListener("submit", submitRecord);
  els.exportCsv.addEventListener("click", exportCsv);
  els.exportXlsx.addEventListener("click", exportXlsx);
  els.importXlsx.addEventListener("change", importXlsx);
  els.cameraInput.addEventListener("change", recognizePhoto);
  
  els.settingsBtn.addEventListener("click", () => {
    els.settingsOverlay.style.display = "flex";
  });
  
  els.closeSettings.addEventListener("click", () => {
    els.settingsOverlay.style.display = "none";
  });

  els.saveSettings.addEventListener("click", saveSettings);
  els.logoutButton.addEventListener("click", logout);
  
  els.settingsOverlay.addEventListener("click", (e) => {
    if (e.target === els.settingsOverlay) els.settingsOverlay.style.display = "none";
  });
}

async function login(e) {
  e.preventDefault();
  const username = els.loginUsername.value.trim();
  const password = els.loginPassword.value;
  if (!username || !password) return;

  els.loginSubmit.disabled = true;
  els.loginSubmit.textContent = "验证中...";

  try {
    const { response: resp, data } = await apiJSON("/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!resp.ok) {
      els.loginError.textContent = data.error || "登录失败";
      return;
    }
    if (data.token) rememberToken(data.token);
    els.loginError.textContent = "";
    els.loginPassword.value = "";
    await showApp();
  } catch {
    els.loginError.textContent = "网络异常";
  } finally {
    els.loginSubmit.disabled = false;
    els.loginSubmit.textContent = "确定";
  }
}

async function logout() {
  await api("/logout", { method: "POST" });
  forgetToken();
  els.settingsOverlay.style.display = "none";
  const { data } = await apiJSON("/check").catch(() => ({ data: {} }));
  showLogin(data);
}

async function loadConfig() {
  try {
    const resp = await api("/config");
    if (!resp.ok) return;
    const cfg = await readJSON(resp);
    els.settingsDataPath.value = cfg.data_path || "";
    els.settingsVisionURL.value = cfg.vision_url || "";
    els.settingsVisionKey.value = cfg.vision_key || "";
    els.settingsVisionModel.value = cfg.vision_model || "";
  } catch { /* ignore */ }
}

async function saveSettings() {
  const payload = {
    data_path: els.settingsDataPath.value.trim(),
    vision_url: els.settingsVisionURL.value.trim(),
    vision_key: els.settingsVisionKey.value.trim(),
    vision_model: els.settingsVisionModel.value.trim(),
    new_password: els.settingsNewPassword.value || null,
    old_password: els.settingsOldPassword.value || null,
  };

  if (!payload.data_path) {
    showSettingsMessage("路径不能为空", true);
    return;
  }

  try {
    const resp = await api("/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await readJSON(resp);
    if (!resp.ok) {
      showSettingsMessage(data.error || "保存失败", true);
      return;
    }
    showSettingsMessage("配置已保存");
    els.settingsOldPassword.value = "";
    els.settingsNewPassword.value = "";
    await loadRecords();
  } catch {
    showSettingsMessage("网络异常", true);
  }
}

function showSettingsMessage(msg, isError = false) {
  els.settingsMessage.textContent = msg;
  els.settingsMessage.style.color = isError ? "var(--accent)" : "var(--primary)";
}

async function loadRecords() {
  try {
    const response = await api("/records");
    if (!response.ok) {
      if (response.status === 401) {
        forgetToken();
        return toLogin();
      }
      return;
    }
    const data = await readJSON(response);
    state.records = data.records || [];
    state.issues = data.issues || [];
    buildMonthOptions();
    render();
  } catch { /* ignore */ }
}

function buildMonthOptions() {
  const months = [...new Set(state.records.map((r) => r.date.slice(0, 7)))].sort();
  if (!state.month && months.length > 0) state.month = months[months.length - 1];
  els.monthSelect.innerHTML = [`<option value="">全部月份</option>`, ...months.map((m) => `<option value="${m}">${m}</option>`)].join("");
  els.monthSelect.value = state.month;
}

function render() {
  const records = filteredRecords();
  renderSummary(records);
  renderCharts(records);
  renderIssues();
  renderList(records);
}

function filteredRecords() {
  return state.records.filter((r) => {
    if (state.month && !r.date.startsWith(state.month)) return false;
    if (state.date && r.date !== state.date) return false;
    if (state.segment !== "all" && r.segment !== state.segment) return false;
    return true;
  });
}

function renderSummary(records) {
  const days = new Set(records.map((r) => r.date));
  const bpRecords = records.filter((r) => r.systolic && r.diastolic);
  const glucoseValues = records.flatMap(v => [v.dynamicGlucose, v.fingerGlucose, v.unknownGlucose].filter(x => x != null));
  const morningGlucose = records.filter(r => r.segment === "早上").flatMap(v => [v.dynamicGlucose, v.fingerGlucose, v.unknownGlucose].filter(x => x != null));

  const avgBP = average(bpRecords.map(r => r.systolic));
  const avgMorning = average(morningGlucose);
  const maxGlucose = glucoseValues.length ? Math.max(...glucoseValues) : null;

  const metrics = [
    { label: "记录天数", value: days.size, detail: `${records.length} 条数据` },
    { label: "平均收缩压", value: avgBP ? `${avgBP.toFixed(0)}` : "-", detail: "mmHg" },
    { label: "晨间血糖均值", value: avgMorning ? `${avgMorning.toFixed(1)}` : "-", detail: "mmol/L" },
    { label: "最高血糖", value: maxGlucose ? `${maxGlucose.toFixed(1)}` : "-", detail: "当前筛选" },
  ];

  els.summary.innerHTML = metrics.map(m => `
    <article class="glass-card metric-card">
      <span>${m.label}</span>
      <strong>${m.value}</strong>
      <small>${m.detail}</small>
    </article>
  `).join("");
}

function renderCharts(records) {
  const byDay = groupBy(records, r => r.date);
  const days = Object.keys(byDay).sort();
  const glucosePoints = days.map(d => ({ date: d, value: average(byDay[d].flatMap(r => [r.dynamicGlucose, r.fingerGlucose, r.unknownGlucose].filter(x => x != null))) })).filter(p => p.value);
  const pressurePoints = days.map(d => ({ date: d, value: average(byDay[d].filter(r => r.systolic).map(r => r.systolic)) })).filter(p => p.value);

  els.rangeLabel.textContent = days.length ? `${days[0]} 至 ${days[days.length - 1]}` : "";
  els.glucoseChart.innerHTML = sparkline(glucosePoints, "var(--glucose)", "mmol/L");
  els.pressureChart.innerHTML = sparkline(pressurePoints, "var(--pressure)", "mmHg");
}

function sparkline(points, color, unit) {
  if (points.length < 2) return `<div class="empty-state">数据不足</div>`;
  const width = 600, height = 160, pad = 20;
  const vals = points.map(p => p.value);
  const min = Math.min(...vals), max = Math.max(...vals), spread = Math.max(max - min, 1);
  const path = points.map((p, i) => {
    const x = pad + (i * (width - pad * 2)) / (points.length - 1);
    const y = height - pad - ((p.value - min) * (height - pad * 2)) / spread;
    return `${i === 0 ? "M" : "L"}${x} ${y}`;
  }).join(" ");
  return `
    <svg viewBox="0 0 ${width} ${height}">
      <path d="${path}" fill="none" stroke="${color}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
      <text x="${pad}" y="${height-5}" fill="var(--text-dim)" font-size="10">${min.toFixed(1)}</text>
      <text x="${pad}" y="15" fill="var(--text-dim)" font-size="10">${max.toFixed(1)}</text>
    </svg>
  `;
}

function renderIssues() {
  els.issues.innerHTML = state.issues.map(i => `<span class="issue-pill">${escapeHtml(i.original)} → ${escapeHtml(i.fixed)}</span>`).join("");
}

function renderList(records) {
  if (!records.length) {
    els.dayTable.innerHTML = document.querySelector("#emptyTemplate").innerHTML;
    return;
  }
  const byDay = groupBy(records, r => r.date);
  const days = Object.keys(byDay).sort().reverse();
  els.dayTable.innerHTML = days.map(d => `
    <div class="day-group">
      <div class="day-header">
        <strong>${d}</strong>
        <span>${weekday(d)} · ${byDay[d].length} 条</span>
      </div>
      ${byDay[d].map(r => `
        <div class="record-item">
          <div class="record-time">${r.time || r.segment}</div>
          <div class="record-data">
            ${r.dynamicGlucose ? `<span class="tag glucose">动态 ${r.dynamicGlucose.toFixed(1)}</span>` : ""}
            ${r.fingerGlucose ? `<span class="tag glucose">扎手 ${r.fingerGlucose.toFixed(1)}</span>` : ""}
            ${r.systolic ? `<span class="tag pressure">血压 ${r.systolic}/${r.diastolic}</span>` : ""}
          </div>
          <button class="btn btn-secondary btn-icon" onclick="deleteRecord('${r.id}')" style="padding: 6px; border-radius: 8px; color: var(--accent);">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
          </button>
        </div>
      `).join("")}
    </div>
  `).join("");
}

async function submitRecord(e) {
  e.preventDefault();
  const fd = new FormData(els.form);
  const record = { date: fd.get("date"), time: fd.get("time"), segment: fd.get("segment"), note: fd.get("note") };
  ["dynamicGlucose", "fingerGlucose", "systolic", "diastolic", "pulse"].forEach(k => { if(fd.get(k)) record[k] = Number(fd.get(k)); });

  try {
    const res = await api("/records", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(record) });
    if (!res.ok) {
      showMessage((await readJSON(res)).error || "保存失败", true);
      return;
    }
    els.form.reset();
    setDefaultFormDate();
    showMessage("记录已添加");
    await loadRecords();
  } catch { showMessage("网络异常", true); }
}

async function deleteRecord(id) {
  if (!confirm("确定删除此条记录吗？")) return;
  try {
    const res = await api(`/records/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (res.ok) {
      showMessage("已删除");
      await loadRecords();
    }
  } catch { showMessage("网络异常", true); }
}

function showMessage(msg, isError = false) {
  els.message.textContent = msg;
  els.message.style.background = isError ? "rgba(244, 63, 94, 0.1)" : "rgba(16, 185, 129, 0.1)";
  els.message.style.color = isError ? "var(--accent)" : "var(--primary)";
  setTimeout(() => els.message.textContent = "", 3000);
}

function exportCsv() {
  const records = filteredRecords();
  const header = ["日期", "时间", "动态血糖", "扎手指", "收缩压", "舒张压", "备注"];
  const rows = records.map(r => [r.date, r.time || "", r.dynamicGlucose || "", r.fingerGlucose || "", r.systolic || "", r.diastolic || "", r.note || ""]);
  const csv = [header, ...rows].map(row => row.map(c => `"${String(c).replace(/"/g, '""')}"`).join(",")).join("\n");
  const blob = new Blob(["\ufeff" + csv], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `xueya-${new Date().toISOString().slice(0,10)}.csv`;
  a.click();
}

function exportXlsx() {
  const params = new URLSearchParams();
  if (state.month) params.set("month", state.month);
  const token = getAuthToken();
  window.location.href = `${apiURL("/records.xlsx", activeApiBase, false)}?${params.toString()}${token ? `&access_token=${token}` : ""}`;
}

async function importXlsx(e) {
  const file = e.target.files[0];
  if (!file) return;
  const fd = new FormData(); fd.append("file", file);
  try {
    const res = await api("/records.xlsx", { method: "POST", body: fd });
    e.target.value = "";
    if (res.ok) {
      const d = await readJSON(res);
      showMessage(`导入成功: ${d.imported || 0} 条`);
      await loadRecords();
    }
  } catch { showMessage("网络异常", true); }
}

async function recognizePhoto(e) {
  const file = e.target.files[0];
  if (!file) return;
  const btn = els.cameraLabel;
  const oldText = btn.innerHTML;
  btn.innerHTML = "识别中...";
  btn.style.opacity = "0.5";
  
  const fd = new FormData(); fd.append("file", file);
  try {
    const res = await api("/recognize", { method: "POST", body: fd });
    e.target.value = "";
    const data = await readJSON(res);
    if (res.ok && data.records?.length) {
      for (const r of data.records) {
        await api("/records", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({...r, date: r.date || new Date().toISOString().slice(0,10), segment: r.segment || "早上"}) });
      }
      showMessage(`识别并保存了 ${data.records.length} 条记录`);
      await loadRecords();
    } else {
      showMessage(data.error || "未识别到数据", true);
    }
  } catch { showMessage("识别失败", true); }
  finally { btn.innerHTML = oldText; btn.style.opacity = "1"; }
}

function setDefaultFormDate() {
  const now = new Date();
  const date = now.toISOString().slice(0, 10);
  const time = now.toTimeString().slice(0, 5);
  if (els.form.elements.date) els.form.elements.date.value = date;
  if (els.form.elements.time) els.form.elements.time.value = time;
}

function weekday(dateText) {
  return ["周日", "周一", "周二", "周三", "周四", "周五", "周六"][new Date(dateText).getDay()];
}

function average(arr) {
  const valid = arr.filter(v => typeof v === "number" && !isNaN(v));
  return valid.length ? valid.reduce((a, b) => a + b, 0) / valid.length : null;
}

function groupBy(arr, fn) {
  return arr.reduce((acc, x) => {
    const k = fn(x);
    (acc[k] = acc[k] || []).push(x);
    return acc;
  }, {});
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]));
}
