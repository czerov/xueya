const segmentOrder = ["早上", "中午", "下午", "晚上"];
const apiBases = ["/_xueya", "/api"];
let activeApiBase = apiBases[0];
const appVersion = "0.1.3";
const requestTimeoutMs = 4500;

function api(url, opts = {}, base = activeApiBase) {
  url = apiURL(url, base);
  opts.credentials = "same-origin";
  try {
    const token = localStorage.getItem("token");
    if (token) {
      opts.headers = { ...opts.headers, Authorization: "Bearer " + token };
    }
  } catch { /* localStorage unavailable */ }
  return fetchWithTimeout(url, opts);
}

function apiURL(url, base = activeApiBase) {
  if (/^https?:\/\//i.test(url)) return url;
  const path = url.startsWith("/api/") ? url.slice(4) : url;
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
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
  if (authBusy) { console.log("checkAuth: busy, skip"); return; }
  authBusy = true;
  try {
    const { data } = await apiJSON("/check");
    console.log("checkAuth:", data);
    setBuildVersion(data);
    if (data.authed) {
      await showApp();
      authBusy = false;
      return;
    }
    showLogin(data);
  } catch(e) {
    console.error("checkAuth error:", e);
    showLogin({
      has_password: true,
      version: appVersion,
      warning: "登录状态接口暂时无响应，可直接尝试登录。",
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
  els.loginHint.textContent = data?.warning || (needsSetup ? "首次使用请设置用户名和密码，后续凭此登录" : "请输入用户名和密码");
  els.loginPassword.autocomplete = needsSetup ? "new-password" : "current-password";
  els.loginUsername.disabled = false;
  els.loginPassword.disabled = false;
  els.loginSubmit.disabled = false;
  els.loginError.textContent = data?.error || "";
  els.loginOverlay.style.display = "flex";
}

function showAuthError(error) {
  els.mainApp.style.display = "none";
  setBuildVersion();
  els.loginTitle.textContent = "后端连接失败";
  els.loginHint.textContent = "请检查容器是否运行最新版本，或打开 /_xueya/version 查看接口状态。";
  els.loginUsername.disabled = false;
  els.loginPassword.disabled = false;
  els.loginSubmit.disabled = false;
  els.loginError.textContent = error?.message || "无法连接后端接口";
  els.loginOverlay.style.display = "flex";
}

function setBuildVersion(data) {
  if (!els.buildVersion) return;
  const version = data?.version || appVersion;
  const commit = data?.commit && data.commit !== "dev" ? ` ${data.commit.slice(0, 7)}` : "";
  els.buildVersion.textContent = `v${version}${commit}`;
}

function toLogin() {
  console.log("toLogin called");
  els.mainApp.style.display = "none";
  els.settingsOverlay.style.display = "none";
  checkAuth();
}

async function showApp() {
  console.log("showApp, token in localStorage:", !!localStorage.getItem("token"));
  els.loginOverlay.style.display = "none";
  els.mainApp.style.display = "";
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

  document.querySelectorAll(".segment-button").forEach((button) => {
    button.addEventListener("click", () => {
      document.querySelectorAll(".segment-button").forEach((item) => item.classList.remove("active"));
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

  els.dayTable.addEventListener("click", async (event) => {
    const button = event.target.closest("[data-delete]");
    if (!button) return;
    await deleteRecord(button.dataset.delete);
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
    console.log("login:", data);
    if (!resp.ok) {
      els.loginError.textContent = data.error || "登录失败";
      return;
    }
    if (data.token) {
      try { localStorage.setItem("token", data.token); } catch {}
    }
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
  localStorage.removeItem("token");
  await api("/logout", { method: "POST" });
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
  const dataPath = els.settingsDataPath.value.trim();
  const visionURL = els.settingsVisionURL.value.trim();
  const visionKey = els.settingsVisionKey.value.trim();
  const visionModel = els.settingsVisionModel.value.trim();
  const newPassword = els.settingsNewPassword.value;
  const oldPassword = els.settingsOldPassword.value;

  if (!dataPath) {
    els.settingsMessage.textContent = "数据路径不能为空";
    els.settingsMessage.style.color = "var(--coral)";
    return;
  }

  try {
    const resp = await api("/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        data_path: dataPath,
        vision_url: visionURL,
        vision_key: visionKey,
        vision_model: visionModel,
        new_password: newPassword || null,
        old_password: oldPassword || null,
      }),
    });
    const data = await readJSON(resp);
    if (!resp.ok) {
      els.settingsMessage.textContent = data.error || "保存失败";
      els.settingsMessage.style.color = "var(--coral)";
      return;
    }
    els.settingsMessage.textContent = newPassword ? "配置和密码已更新" : "配置已保存";
    els.settingsMessage.style.color = "var(--teal)";
    els.settingsOldPassword.value = "";
    els.settingsNewPassword.value = "";
    await loadRecords();
  } catch {
    els.settingsMessage.textContent = "网络异常";
    els.settingsMessage.style.color = "var(--coral)";
  }
}

async function loadRecords() {
  try {
    const response = await api("/records");
    console.log("loadRecords status:", response.status);
    if (!response.ok) {
      if (response.status === 401) { localStorage.removeItem("token"); return toLogin(); }
      return;
    }
    const data = await readJSON(response);
    state.records = data.records || [];
    state.issues = data.issues || [];
    buildMonthOptions();
    setDefaultFormDate();
    render();
  } catch { /* ignore */ }
}

function buildMonthOptions() {
  const months = [...new Set(state.records.map((record) => record.date.slice(0, 7)))].sort();
  if (!state.month && months.length > 0) {
    state.month = months[months.length - 1];
  }

  els.monthSelect.innerHTML = [
    `<option value="">全部月份</option>`,
    ...months.map((month) => `<option value="${month}">${month}</option>`),
  ].join("");
  els.monthSelect.value = state.month;
}

function render() {
  const records = filteredRecords();
  renderSummary(records);
  renderCharts(records);
  renderIssues();
  renderDayTable(records);
}

function filteredRecords() {
  return state.records.filter((record) => {
    if (state.month && !record.date.startsWith(state.month)) return false;
    if (state.date && record.date !== state.date) return false;
    if (state.segment !== "all" && record.segment !== state.segment) return false;
    return true;
  });
}

function renderSummary(records) {
  const days = new Set(records.map((record) => record.date));
  const bpRecords = records.filter((record) => record.systolic && record.diastolic);
  const glucoseValues = records.flatMap(valuesFromRecord);
  const morningValues = records.filter((record) => record.segment === "早上").flatMap(valuesFromRecord);
  const maxGlucose = glucoseValues.length ? Math.max(...glucoseValues) : null;
  const avgBP = average(bpRecords.map((record) => record.systolic));
  const avgMorning = average(morningValues);

  const cards = [
    ["记录天数", days.size || "0", `${records.length} 条记录`],
    ["平均收缩压", avgBP ? `${avgBP.toFixed(0)} mmHg` : "无", `${bpRecords.length} 次血压`],
    ["早上血糖均值", avgMorning ? `${avgMorning.toFixed(1)} mmol/L` : "无", `${morningValues.length} 个血糖值`],
    ["最高血糖", maxGlucose ? `${maxGlucose.toFixed(1)} mmol/L` : "无", "按当前筛选统计"],
  ];

  els.summary.innerHTML = cards
    .map(([label, value, detail]) => `
      <article class="metric-card">
        <span>${label}</span>
        <strong>${value}</strong>
        <small>${detail}</small>
      </article>
    `)
    .join("");
}

function renderCharts(records) {
  const byDay = groupBy(records, (record) => record.date);
  const days = Object.keys(byDay).sort();
  const glucosePoints = days.map((date) => ({
    date,
    value: average(byDay[date].flatMap(valuesFromRecord)),
  })).filter((point) => point.value);
  const pressurePoints = days.map((date) => ({
    date,
    value: average(byDay[date].filter((record) => record.systolic).map((record) => record.systolic)),
  })).filter((point) => point.value);

  els.rangeLabel.textContent = days.length ? `${days[0]} 至 ${days[days.length - 1]}` : "";
  els.glucoseChart.innerHTML = sparkline(glucosePoints, "#236b64", "mmol/L");
  els.pressureChart.innerHTML = sparkline(pressurePoints, "#c95646", "mmHg");
}

function sparkline(points, color, unit) {
  if (points.length < 2) {
    return `<div class="empty-state"><span>数据点不足</span></div>`;
  }
  const width = 620;
  const height = 178;
  const padding = 24;
  const values = points.map((point) => point.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const spread = Math.max(max - min, 1);
  const path = points.map((point, index) => {
    const x = padding + (index * (width - padding * 2)) / Math.max(points.length - 1, 1);
    const y = height - padding - ((point.value - min) * (height - padding * 2)) / spread;
    return `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
  }).join(" ");
  const last = points[points.length - 1];

  return `
    <svg viewBox="0 0 ${width} ${height}" role="img" aria-label="趋势图">
      <path d="M${padding} ${height - padding} H${width - padding}" stroke="#cfdeda" stroke-width="1" />
      <path d="${path}" fill="none" stroke="${color}" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" />
      <circle cx="${width - padding}" cy="${height - padding - ((last.value - min) * (height - padding * 2)) / spread}" r="5" fill="${color}" />
      <text x="${padding}" y="22" fill="#687776" font-size="15">${min.toFixed(1)} - ${max.toFixed(1)} ${unit}</text>
      <text x="${width - padding - 120}" y="22" fill="${color}" font-size="15">最新 ${last.value.toFixed(1)}</text>
    </svg>
  `;
}

function renderIssues() {
  if (!state.issues.length) {
    els.issues.innerHTML = "";
    return;
  }
  els.issues.innerHTML = state.issues
    .map((issue) => `<span class="issue-pill">${escapeHtml(issue.original)} → ${escapeHtml(issue.fixed)}：${escapeHtml(issue.message)}</span>`)
    .join("");
}

function renderDayTable(records) {
  if (!records.length) {
    els.dayTable.innerHTML = document.querySelector("#emptyTemplate").innerHTML;
    return;
  }
  const byDay = groupBy(records, (record) => record.date);
  const days = Object.keys(byDay).sort().reverse();
  els.dayTable.innerHTML = days.map((date) => dayRow(date, byDay[date])).join("");
}

function dayRow(date, records) {
  const bySegment = groupBy(records, (record) => record.segment || "未分段");
  const readingsCount = records.length;
  return `
    <article class="day-row">
      <div class="date-cell">
        <strong>${date}</strong>
        <span>${weekday(date)} · ${readingsCount} 条</span>
      </div>
      ${segmentOrder.map((segment) => `
        <div class="segment-cell" data-label="${segment}">
          ${(bySegment[segment] || []).map(readingMarkup).join("") || `<span class="empty-slot">无记录</span>`}
        </div>
      `).join("")}
    </article>
  `;
}

function readingMarkup(record) {
  const tags = [];
  if (record.dynamicGlucose != null) tags.push(glucoseTag("动态", record.dynamicGlucose));
  if (record.fingerGlucose != null) tags.push(glucoseTag("扎手", record.fingerGlucose));
  if (record.unknownGlucose != null) tags.push(glucoseTag("血糖", record.unknownGlucose));
  if (record.systolic && record.diastolic) tags.push(pressureTag(record));

  return `
    <div class="reading">
      <div class="reading-time">${record.time || record.label || record.segment}</div>
      <div class="tag-row">${tags.join("")}</div>
      ${record.note ? `<div class="note">${escapeHtml(record.note)}</div>` : ""}
      ${record.source === "manual" ? `<button class="delete-button" type="button" data-delete="${record.id}">删除</button>` : ""}
    </div>
  `;
}

function glucoseTag(label, value) {
  const tone = value >= 10 ? "warn" : value <= 3.9 ? "low" : "";
  return `<span class="tag ${tone}">${label} ${Number(value).toFixed(1)}</span>`;
}

function pressureTag(record) {
  const high = record.systolic >= 140 || record.diastolic >= 90;
  const low = record.systolic < 90 || record.diastolic < 60;
  const tone = high ? "warn" : low ? "low" : "pressure";
  return `<span class="tag ${tone}">血压 ${record.systolic}/${record.diastolic} · ${record.pulse || "-"}</span>`;
}

async function submitRecord(event) {
  event.preventDefault();
  const form = new FormData(els.form);
  const record = {
    date: form.get("date"),
    time: form.get("time"),
    segment: form.get("segment"),
    note: form.get("note"),
  };

  addNumber(record, "dynamicGlucose", form.get("dynamicGlucose"));
  addNumber(record, "fingerGlucose", form.get("fingerGlucose"));
  addNumber(record, "systolic", form.get("systolic"));
  addNumber(record, "diastolic", form.get("diastolic"));
  addNumber(record, "pulse", form.get("pulse"));

  try {
    const response = await api("/records", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(record),
    });

    if (!response.ok) {
      const error = await readJSON(response).catch(() => ({ error: "保存失败" }));
      showMessage(error.error || "保存失败", true);
      return;
    }

    els.form.reset();
    setDefaultFormDate();
    state.month = record.date.slice(0, 7);
    showMessage("已保存");
    await loadRecords();
  } catch {
    showMessage("网络异常", true);
  }
}

async function deleteRecord(id) {
  try {
    const response = await api(`/records/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!response.ok) {
      showMessage("删除失败", true);
      return;
    }
    showMessage("已删除");
    await loadRecords();
  } catch {
    showMessage("网络异常", true);
  }
}

function exportCsv() {
  const records = filteredRecords();
  const header = ["日期", "时间", "时间段", "动态血糖", "扎手指血糖", "未标注血糖", "收缩压", "舒张压", "心率", "备注"];
  const rows = records.map((record) => [
    record.date,
    record.time || "",
    record.segment || "",
    record.dynamicGlucose ?? "",
    record.fingerGlucose ?? "",
    record.unknownGlucose ?? "",
    record.systolic ?? "",
    record.diastolic ?? "",
    record.pulse ?? "",
    record.note || "",
  ]);
  const csv = [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
  const blob = new Blob([`\ufeff${csv}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `xueya-${state.month || "all"}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

function exportXlsx() {
  window.location.href = `${apiURL("/records.xlsx")}?${filterParams().toString()}`;
}

async function importXlsx(event) {
  const file = event.target.files?.[0];
  if (!file) return;

  const form = new FormData();
  form.append("file", file);
  try {
    const response = await api("/records.xlsx", {
      method: "POST",
      body: form,
    });
    event.target.value = "";

    const result = await readJSON(response).catch(() => ({}));
    if (!response.ok) {
      showMessage(result.error || "导入失败", true);
      return;
    }

    showMessage(`已导入 ${result.imported || 0} 条，跳过 ${result.skipped || 0} 条`);
    await loadRecords();
  } catch {
    showMessage("网络异常", true);
  }
}

async function recognizePhoto(event) {
  const file = event.target.files?.[0];
  if (!file) return;

  const label = els.cameraLabel;
  const originalText = label.textContent;
  label.textContent = "识别中...";
  label.style.pointerEvents = "none";

  const formData = new FormData();
  formData.append("file", file);

  try {
    const response = await api("/recognize", {
      method: "POST",
      body: formData,
    });
    event.target.value = "";

    const payload = await readJSON(response);
    if (!response.ok) {
      showMessage(payload.error || "识别失败", true);
      return;
    }

    const records = payload.records || [];
    if (!records.length) {
      showMessage("未识别到任何数值", true);
      return;
    }

    let saved = 0;
    for (const item of records) {
      const record = buildRecord(item);
      const res = await api("/records", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(record),
      });
      if (res.ok) saved++;
    }

    els.form.reset();
    setDefaultFormDate();
    showMessage(`识别 ${records.length} 条，已自动保存 ${saved} 条`);
    await loadRecords();
  } catch {
    showMessage("网络异常，识别失败", true);
  } finally {
    label.textContent = originalText;
    label.style.pointerEvents = "";
  }
}

function buildRecord(data) {
  const today = new Date().toISOString().slice(0, 10);
  return {
    date: data.date || els.form.elements.date.value || today,
    time: data.time || els.form.elements.time.value || null,
    segment: els.form.elements.segment.value,
    dynamicGlucose: data.dynamicGlucose ?? null,
    fingerGlucose: data.fingerGlucose ?? null,
    systolic: data.systolic ?? null,
    diastolic: data.diastolic ?? null,
    pulse: data.pulse ?? null,
  };
}

function filterParams() {
  const params = new URLSearchParams();
  if (state.month) params.set("month", state.month);
  if (state.date) params.set("date", state.date);
  if (state.segment && state.segment !== "all") params.set("segment", state.segment);
  return params;
}

function addNumber(target, key, value) {
  if (value !== "") target[key] = Number(value);
}

function valuesFromRecord(record) {
  return [record.dynamicGlucose, record.fingerGlucose, record.unknownGlucose]
    .filter((value) => value != null)
    .map(Number);
}

function average(values) {
  const usable = values.filter((value) => Number.isFinite(value));
  if (!usable.length) return null;
  return usable.reduce((sum, value) => sum + value, 0) / usable.length;
}

function groupBy(items, getKey) {
  return items.reduce((groups, item) => {
    const key = getKey(item);
    groups[key] ||= [];
    groups[key].push(item);
    return groups;
  }, {});
}

function weekday(dateText) {
  return new Intl.DateTimeFormat("zh-CN", { weekday: "short" }).format(new Date(`${dateText}T00:00:00`));
}

function setDefaultFormDate() {
  const latest = state.records.map((record) => record.date).sort().at(-1);
  els.form.elements.date.value = latest || new Date().toISOString().slice(0, 10);
}

function showMessage(message, isError = false) {
  els.message.textContent = message;
  els.message.style.color = isError ? "var(--coral)" : "var(--teal)";
  clearTimeout(showMessage.timer);
  showMessage.timer = setTimeout(() => {
    els.message.textContent = "";
  }, 2600);
}

function csvCell(value) {
  const text = String(value ?? "");
  if (/[",\n]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`;
  }
  return text;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}
