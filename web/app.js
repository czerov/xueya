const segmentOrder = ["早上", "中午", "下午", "晚上"];

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
  dataPathInput: document.querySelector("#dataPathInput"),
  saveConfig: document.querySelector("#saveConfig"),
  configMessage: document.querySelector("#configMessage"),
  toggleConfig: document.querySelector("#toggleConfig"),
  configBar: document.querySelector("#configBar"),
  cameraInput: document.querySelector("#cameraInput"),
  cameraLabel: document.querySelector(".camera-button"),
};

init();

async function init() {
  setDefaultFormDate();
  bindEvents();
  await loadConfig();
  await loadRecords();
}

function bindEvents() {
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

  els.saveConfig.addEventListener("click", saveConfig);
  els.toggleConfig.addEventListener("click", () => {
    const body = els.configBar.querySelector("label");
    const btn = els.toggleConfig;
    const hidden = body.style.display === "none";
    body.style.display = hidden ? "" : "none";
    btn.textContent = hidden ? "▲" : "▼";
  });

  els.dayTable.addEventListener("click", async (event) => {
    const button = event.target.closest("[data-delete]");
    if (!button) return;
    await deleteRecord(button.dataset.delete);
  });
}

async function loadRecords() {
  const response = await fetch("/api/records");
  if (!response.ok) {
    showMessage("数据读取失败", true);
    return;
  }
  const data = await response.json();
  state.records = data.records || [];
  state.issues = data.issues || [];
  buildMonthOptions();
  setDefaultFormDate();
  render();
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

  const response = await fetch("/api/records", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(record),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "保存失败" }));
    showMessage(error.error || "保存失败", true);
    return;
  }

  els.form.reset();
  setDefaultFormDate();
  state.month = record.date.slice(0, 7);
  showMessage("已保存");
  await loadRecords();
}

async function deleteRecord(id) {
  const response = await fetch(`/api/records/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!response.ok) {
    showMessage("删除失败", true);
    return;
  }
  showMessage("已删除");
  await loadRecords();
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
  window.location.href = `/api/records.xlsx?${filterParams().toString()}`;
}

async function importXlsx(event) {
  const file = event.target.files?.[0];
  if (!file) return;

  const form = new FormData();
  form.append("file", file);
  const response = await fetch("/api/records.xlsx", {
    method: "POST",
    body: form,
  });
  event.target.value = "";

  const result = await response.json().catch(() => ({}));
  if (!response.ok) {
    showMessage(result.error || "导入失败", true);
    return;
  }

  showMessage(`已导入 ${result.imported || 0} 条，跳过 ${result.skipped || 0} 条`);
  await loadRecords();
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
    const response = await fetch("/api/recognize", {
      method: "POST",
      body: formData,
    });
    event.target.value = "";

    const payload = await response.json();
    if (!response.ok) {
      showMessage(payload.error || "识别失败", true);
      return;
    }

    const records = payload.records || [];
    if (!records.length) {
      showMessage("未识别到任何数值", true);
      return;
    }

    const first = records[0];
    if (first.dynamicGlucose != null) els.form.elements.dynamicGlucose.value = first.dynamicGlucose;
    if (first.fingerGlucose != null) els.form.elements.fingerGlucose.value = first.fingerGlucose;
    if (first.systolic != null) els.form.elements.systolic.value = first.systolic;
    if (first.diastolic != null) els.form.elements.diastolic.value = first.diastolic;
    if (first.pulse != null) els.form.elements.pulse.value = first.pulse;

    if (records.length > 1) {
      let saved = 0;
      for (let i = 1; i < records.length; i++) {
        const record = buildRecord(records[i]);
        const res = await fetch("/api/records", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(record),
        });
        if (res.ok) saved++;
      }
      showMessage(`识别 ${records.length} 条，已自动保存 ${saved} 条，第1条已填入表单`);
    } else {
      showMessage("识别成功，请核对数据");
    }
  } catch {
    showMessage("网络异常，识别失败", true);
  } finally {
    label.textContent = originalText;
    label.style.pointerEvents = "";
  }
}

function buildRecord(data) {
  return {
    date: els.form.elements.date.value,
    time: els.form.elements.time.value || null,
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

async function loadConfig() {
  const response = await fetch("/api/config");
  if (!response.ok) return;
  const cfg = await response.json();
  els.dataPathInput.value = cfg.data_path || "";
}

async function saveConfig() {
  const dataPath = els.dataPathInput.value.trim();
  if (!dataPath) {
    showConfigMessage("数据路径不能为空", true);
    return;
  }

  const response = await fetch("/api/config", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ data_path: dataPath }),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "保存配置失败" }));
    showConfigMessage(error.error || "保存配置失败", true);
    return;
  }

  showConfigMessage("配置已保存，数据已刷新");
  await loadRecords();
}

function showConfigMessage(message, isError = false) {
  els.configMessage.textContent = message;
  els.configMessage.style.color = isError ? "var(--coral)" : "var(--teal)";
  clearTimeout(showConfigMessage.timer);
  showConfigMessage.timer = setTimeout(() => {
    els.configMessage.textContent = "";
  }, 3000);
}
