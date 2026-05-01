(function() {
  var segmentOrder = ["早上", "中午", "下午", "晚上"];
  var apiBases = ["/_xueya", "/api"];
  var activeApiBase = apiBases[0];
  var authToken = "";
  var appVersion = "0.1.5";
  var requestTimeoutMs = 15000;

  var state = {
    records: [],
    issues: [],
    month: "",
    date: "",
    segment: "all"
  };

  var els = {};

  function init() {
    initElements();
    initTheme();
    bindAuthEvents();
    getAuthToken();
    checkAuth();
  }

  function initTheme() {
    var theme = localStorage.getItem("theme") || "dark";
    document.documentElement.setAttribute("data-theme", theme);
    updateThemeIcon(theme);
    if (els.themeToggle) {
      els.themeToggle.onclick = toggleTheme;
    }
  }

  function toggleTheme() {
    var current = document.documentElement.getAttribute("data-theme") || "dark";
    var target = current === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", target);
    localStorage.setItem("theme", target);
    updateThemeIcon(target);
  }

  function updateThemeIcon(theme) {
    if (!els.themeToggle) return;
    var moon = '<path d="M12 3c-4.97 0-9 4.03-9 9s4.03 9 9 9 9-4.03 9-9c0-.46-.04-.92-.1-1.36-.98 1.37-2.58 2.26-4.4 2.26-3.03 0-5.5-2.47-5.5-5.5 0-1.82.89-3.42 2.26-4.4-.44-.06-.9-.1-1.36-.1z"/>';
    var sun = '<path d="M12 7c-2.76 0-5 2.24-5 5s2.24 5 5 5 5-2.24 5-5-2.24-5-5-5zM2 13h2c.55 0 1-.45 1-1s-.45-1-1-1H2c-.55 0-1 .45-1 1s.45 1 1 1zm18 0h2c.55 0 1-.45 1-1s-.45-1-1-1h-2c-.55 0-1 .45-1 1s.45 1 1 1zM11 2v2c0 .55.45 1 1 1s1-.45 1-1V2c0-.55-.45-1-1-1s-1 .45-1 1zm0 18v2c0 .55.45 1 1 1s1-.45 1-1v-2c0-.55-.45-1-1-1s-1 .45-1 1zM5.99 4.58a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41L5.99 4.58zm12.37 12.37a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41l-1.06-1.06zm1.06-10.96a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06zM7.05 18.37a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06z"/>';
    els.themeToggle.innerHTML = '<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">' + (theme === 'dark' ? moon : sun) + '</svg>';
  }

  function initElements() {
    var ids = [
      "monthSelect", "dateSearch", "summary", "dayTable", "issues", "rangeLabel",
      "glucoseChart", "pressureChart", "recordForm", "message", "exportCsv",
      "exportXlsx", "importXlsx", "cameraInput", "loginOverlay", "loginForm",
      "loginTitle", "loginHint", "loginUsername", "loginPassword", "loginSubmit",
      "loginError", "buildVersion", "mainApp", "settingsBtn", "settingsOverlay",
      "settingsDataPath", "settingsVisionURL", "settingsVisionKey", "settingsVisionModel",
      "settingsOldPassword", "settingsNewPassword", "saveSettings", "settingsMessage",
      "closeSettings", "logoutButton", "themeToggle"
    ];
    for (var i = 0; i < ids.length; i++) {
      els[ids[i]] = document.getElementById(ids[i]);
    }
    els.cameraLabel = document.querySelector(".camera-button");
    els.form = els.recordForm;
  }

  function api(url, opts, base) {
    if (!opts) opts = {};
    if (!base) base = activeApiBase;
    url = apiURL(url, base);
    opts.credentials = "same-origin";
    var token = getAuthToken();
    if (token) {
      if (!opts.headers) opts.headers = {};
      opts.headers["Authorization"] = "Bearer " + token;
    }
    return fetchWithTimeout(url, opts);
  }

  function apiURL(url, base, includeToken) {
    if (includeToken === undefined) includeToken = true;
    if (url.indexOf("http") === 0) return url;
    var path = url.indexOf("/api/") === 0 ? url.slice(4) : url;
    var full = base + (path.indexOf("/") === 0 ? path : "/" + path);
    var token = includeToken ? getAuthToken() : "";
    if (!token) return full;
    var separator = full.indexOf("?") !== -1 ? "&" : "?";
    return full + separator + "access_token=" + encodeURIComponent(token);
  }

  function fetchWithTimeout(url, opts) {
    var controller = new AbortController();
    var timer = setTimeout(function() { controller.abort(); }, requestTimeoutMs);
    opts.signal = controller.signal;
    return fetch(url, opts).then(function(res) {
      clearTimeout(timer);
      return res;
    })["catch"](function(err) {
      clearTimeout(timer);
      throw err;
    });
  }

  function readJSON(response) {
    return response.text().then(function(text) {
      try {
        return text ? JSON.parse(text) : {};
      } catch (err) {
        throw new Error("JSON Parse Error: " + response.status);
      }
    });
  }

  function apiJSON(url, opts) {
    var lastError;
    var bases = [activeApiBase];
    for (var i = 0; i < apiBases.length; i++) {
      if (apiBases[i] !== activeApiBase) bases.push(apiBases[i]);
    }

    function tryBase(index) {
      if (index >= bases.length) throw lastError || new Error("Connection Failed");
      var base = bases[index];
      return api(url, opts, base).then(function(res) {
        return readJSON(res).then(function(data) {
          activeApiBase = base;
          return { response: res, data: data };
        });
      })["catch"](function(err) {
        lastError = err;
        return tryBase(index + 1);
      });
    }
    return tryBase(0);
  }

  function getAuthToken() {
    if (location.hash.indexOf("#token=") === 0) {
      var t = decodeURIComponent(location.hash.slice(7));
      if (t) {
        authToken = t;
        rememberToken(t);
        history.replaceState(null, "", location.pathname + location.search);
        return t;
      }
    }
    if (authToken) return authToken;
    try { authToken = localStorage.getItem("token") || ""; } catch(e) {}
    return authToken;
  }

  function rememberToken(t) {
    authToken = t || "";
    if (!authToken) return;
    try { localStorage.setItem("token", t); } catch(e) {}
  }

  function forgetToken() {
    authToken = "";
    try { localStorage.removeItem("token"); } catch(e) {}
    if (location.hash.indexOf("#token=") === 0) {
      history.replaceState(null, "", location.pathname + location.search);
    }
  }

  function checkAuth() {
    apiJSON("/check").then(function(res) {
      var data = res.data;
      setBuildVersion(data);
      if (data && data.authed) {
        showApp();
      } else {
        showLogin(data);
      }
    })["catch"](function(err) {
      showLogin({ error: err.message, has_password: true });
    });
  }

  function showLogin(data) {
    if (els.mainApp) els.mainApp.style.display = "none";
    if (els.loginOverlay) {
      els.loginOverlay.style.display = "flex";
      els.loginOverlay.style.opacity = "1";
    }
    setBuildVersion(data);
    var needsSetup = data ? !data.has_password : false;
    if (els.loginTitle) els.loginTitle.textContent = needsSetup ? "设置访问密码" : "登录";
    if (els.loginHint) els.loginHint.textContent = (data && data.warning) || (needsSetup ? "首次使用请设置访问凭据" : "请输入凭据以继续");
  }

  function showApp() {
    if (els.loginOverlay) els.loginOverlay.style.display = "none";
    if (els.mainApp) {
      els.mainApp.style.display = "block";
      els.mainApp.style.opacity = "1";
      els.mainApp.classList.add("ready");
    }
    setDefaultFormDate();
    bindEvents();
    loadConfig();
    loadRecords();
  }

  function bindAuthEvents() {
    if (els.loginForm) {
      els.loginForm.onsubmit = function(e) {
        e.preventDefault();
        login();
      };
    }
  }

  function login() {
    var u = els.loginUsername.value.trim();
    var p = els.loginPassword.value;
    if (!u || !p) return;
    els.loginSubmit.disabled = true;
    els.loginSubmit.textContent = "验证中...";
    apiJSON("/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: u, password: p })
    }).then(function(res) {
      if (res.response.ok) {
        if (res.data.token) rememberToken(res.data.token);
        showApp();
      } else {
        els.loginError.textContent = res.data.error || "登录失败";
      }
    })["catch"](function() {
      els.loginError.textContent = "网络异常";
    }).then(function() {
      els.loginSubmit.disabled = false;
      els.loginSubmit.textContent = "确定";
    });
  }

  function loadRecords() {
    api("/records").then(function(res) {
      if (res.status === 401) {
        forgetToken();
        location.reload();
        return;
      }
      return readJSON(res).then(function(data) {
        state.records = data.records || [];
        state.issues = data.issues || [];
        buildMonthOptions();
        render();
      });
    });
  }

  function buildMonthOptions() {
    var raw = [];
    for (var i = 0; i < state.records.length; i++) {
      var m = state.records[i].date.slice(0, 7);
      if (raw.indexOf(m) === -1) raw.push(m);
    }
    raw.sort();
    if (!state.month && raw.length > 0) state.month = raw[raw.length - 1];
    var html = '<option value="">全部月份</option>';
    for (var j = 0; j < raw.length; j++) {
      html += '<option value="' + raw[j] + '">' + raw[j] + '</option>';
    }
    els.monthSelect.innerHTML = html;
    els.monthSelect.value = state.month;
  }

  function render() {
    var records = state.records.filter(function(r) {
      if (state.month && r.date.indexOf(state.month) !== 0) return false;
      if (state.date && r.date !== state.date) return false;
      if (state.segment !== "all" && r.segment !== state.segment) return false;
      return true;
    });
    renderSummary(records);
    renderCharts(records);
    renderIssues();
    renderList(records);
  }

  function renderSummary(records) {
    var dayMap = {};
    var bpVals = [];
    var glVals = [];
    var morningGl = [];
    for (var i = 0; i < records.length; i++) {
      var r = records[i];
      dayMap[r.date] = true;
      if (r.systolic) bpVals.push(r.systolic);
      var gs = [r.dynamicGlucose, r.fingerGlucose, r.unknownGlucose];
      for (var j = 0; j < gs.length; j++) {
        if (gs[j] != null) {
          glVals.push(gs[j]);
          if (r.segment === "早上") morningGl.push(gs[j]);
        }
      }
    }
    var dayCount = Object.keys(dayMap).length;
    var avgBP = average(bpVals);
    var avgMorn = average(morningGl);
    var maxGl = glVals.length ? Math.max.apply(null, glVals) : 0;

    els.summary.innerHTML = 
      '<article class="glass-card metric-card"><span>记录天数</span><strong>' + dayCount + '</strong><small>' + records.length + ' 条数据</small></article>' +
      '<article class="glass-card metric-card"><span>平均收缩压</span><strong>' + (avgBP ? avgBP.toFixed(0) : "-") + '</strong><small>mmHg</small></article>' +
      '<article class="glass-card metric-card"><span>晨间血糖均值</span><strong>' + (avgMorn ? avgMorn.toFixed(1) : "-") + '</strong><small>mmol/L</small></article>' +
      '<article class="glass-card metric-card"><span>最高血糖</span><strong>' + (maxGl ? maxGl.toFixed(1) : "-") + '</strong><small>当前筛选</small></article>';
  }

  function renderCharts(records) {
    var byDay = {};
    for (var i = 0; i < records.length; i++) {
      var r = records[i];
      if (!byDay[r.date]) byDay[r.date] = [];
      byDay[r.date].push(r);
    }
    var days = Object.keys(byDay).sort();
    var glPoints = [];
    var bpPoints = [];
    for (var j = 0; j < days.length; j++) {
      var d = days[j];
      var dRecords = byDay[d];
      var dGl = [];
      var dBp = [];
      for (var k = 0; k < dRecords.length; k++) {
        var dr = dRecords[k];
        if (dr.systolic) dBp.push(dr.systolic);
        var dgs = [dr.dynamicGlucose, dr.fingerGlucose, dr.unknownGlucose];
        for (var l = 0; l < dgs.length; l++) if (dgs[l] != null) dGl.push(dgs[l]);
      }
      var ga = average(dGl); if (ga) glPoints.push({ value: ga });
      var ba = average(dBp); if (ba) bpPoints.push({ value: ba });
    }
    els.rangeLabel.textContent = days.length ? (days[0] + " 至 " + days[days.length - 1]) : "";
    els.glucoseChart.innerHTML = sparkline(glPoints, "var(--glucose)");
    els.pressureChart.innerHTML = sparkline(bpPoints, "var(--pressure)");
  }

  function sparkline(points, color) {
    if (points.length < 2) return '<div class="empty-state">数据不足</div>';
    var w = 600, h = 160, p = 20;
    var vals = points.map(function(pt) { return pt.value; });
    var min = Math.min.apply(null, vals), max = Math.max.apply(null, vals), sp = Math.max(max - min, 1);
    var path = "";
    for (var i = 0; i < points.length; i++) {
      var x = p + (i * (w - p * 2)) / (points.length - 1);
      var y = h - p - ((points[i].value - min) * (h - p * 2)) / sp;
      path += (i === 0 ? "M" : "L") + x + " " + y;
    }
    return '<svg viewBox="0 0 ' + w + ' ' + h + '"><path d="' + path + '" fill="none" stroke="' + color + '" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" /></svg>';
  }

  function renderList(records) {
    var byDay = {};
    for (var i = 0; i < records.length; i++) {
      var r = records[i];
      if (!byDay[r.date]) byDay[r.date] = [];
      byDay[r.date].push(r);
    }
    var days = Object.keys(byDay).sort().reverse();
    var html = "";
    if (days.length === 0) {
      els.dayTable.innerHTML = document.getElementById("emptyTemplate").innerHTML;
      return;
    }
    for (var j = 0; j < days.length; j++) {
      var d = days[j];
      html += '<div class="day-group"><div class="day-header"><strong>' + d + '</strong><span>' + weekday(d) + '</span></div>';
      var drs = byDay[d];
      for (var k = 0; k < drs.length; k++) {
        var dr = drs[k];
        html += '<div class="record-item"><div class="record-time">' + (dr.time || dr.segment) + '</div><div class="record-data">';
        if (dr.dynamicGlucose) html += '<span class="tag glucose">动态 ' + dr.dynamicGlucose.toFixed(1) + '</span>';
        if (dr.fingerGlucose) html += '<span class="tag glucose">扎手 ' + dr.fingerGlucose.toFixed(1) + '</span>';
        if (dr.systolic) html += '<span class="tag pressure">血压 ' + dr.systolic + '/' + dr.diastolic + '</span>';
        html += '</div><button class="btn btn-secondary btn-icon" onclick="deleteRecord(\'' + dr.id + '\')" style="color:var(--accent)"><svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg></button></div>';
      }
      html += '</div>';
    }
    els.dayTable.innerHTML = html;
  }

  window.deleteRecord = function(id) {
    if (!confirm("确定删除吗？")) return;
    api("/records/" + encodeURIComponent(id), { method: "DELETE" }).then(function(res) {
      if (res.ok) loadRecords();
    });
  };

  function bindEvents() {
    els.monthSelect.onchange = function() { state.month = els.monthSelect.value; render(); };
    els.dateSearch.onchange = function() { state.date = els.dateSearch.value; render(); };
    var btns = document.querySelectorAll(".segment-btn");
    for (var i = 0; i < btns.length; i++) {
      (function(b) {
        b.onclick = function() {
          for (var j = 0; j < btns.length; j++) btns[j].classList.remove("active");
          b.classList.add("active");
          state.segment = b.getAttribute("data-segment");
          render();
        };
      })(btns[i]);
    }
    els.recordForm.onsubmit = function(e) {
      e.preventDefault();
      var fd = new FormData(els.recordForm);
      var rec = { date: fd.get("date"), time: fd.get("time"), segment: fd.get("segment"), note: fd.get("note") };
      var ks = ["dynamicGlucose", "fingerGlucose", "systolic", "diastolic", "pulse"];
      for (var k = 0; k < ks.length; k++) if (fd.get(ks[k])) rec[ks[k]] = Number(fd.get(ks[k]));
      apiJSON("/records", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(rec) }).then(function(res) {
        if (res.response.ok) { els.recordForm.reset(); setDefaultFormDate(); loadRecords(); showMessage("已保存"); }
      });
    };
    els.settingsBtn.onclick = function() { els.settingsOverlay.style.display = "flex"; };
    els.closeSettings.onclick = function() { els.settingsOverlay.style.display = "none"; };
    els.logoutButton.onclick = function() { api("/logout", { method: "POST" }).then(function() { forgetToken(); location.reload(); }); };
    els.exportCsv.onclick = exportCsv;
    els.exportXlsx.onclick = function() {
      var token = getAuthToken();
      var url = apiURL("/records.xlsx", activeApiBase, false) + "?month=" + state.month + (token ? "&access_token=" + token : "");
      window.location.href = url;
    };
  }

  function exportCsv() {
    var header = ["日期", "时间", "血糖", "收缩压", "舒张压"];
    var csv = "\ufeff" + header.join(",") + "\n";
    for (var i = 0; i < state.records.length; i++) {
      var r = state.records[i];
      csv += [r.date, r.time || "", r.dynamicGlucose || "", r.systolic || "", r.diastolic || ""].join(",") + "\n";
    }
    var blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
    var a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "xueya.csv";
    a.click();
  }

  function showMessage(msg) {
    els.message.textContent = msg;
    setTimeout(function() { els.message.textContent = ""; }, 3000);
  }

  function setDefaultFormDate() {
    var d = new Date();
    els.recordForm.elements.date.value = d.toISOString().slice(0, 10);
    els.recordForm.elements.time.value = d.toTimeString().slice(0, 5);
  }

  function average(arr) {
    var v = arr.filter(function(x) { return typeof x === "number" && !isNaN(x); });
    return v.length ? v.reduce(function(a, b) { return a + b; }, 0) / v.length : null;
  }

  function weekday(dt) {
    return ["周日", "周一", "周二", "周三", "周四", "周五", "周六"][new Date(dt).getDay()];
  }

  function setBuildVersion(data) {
    var v = (data && data.version) || appVersion;
    if (els.buildVersion) els.buildVersion.textContent = "v" + v;
  }

  function renderIssues() {
    if (!els.issues) return;
    els.issues.innerHTML = state.issues.map(function(i) { return '<span class="issue-pill">' + i.original + '</span>'; }).join("");
  }

  function loadConfig() {
    api("/config").then(function(res) {
      if (res.ok) return readJSON(res).then(function(c) {
        els.settingsDataPath.value = c.data_path || "";
        els.settingsVisionURL.value = c.vision_url || "";
      });
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, function(m) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]; });
  }

  if (document.readyState === "complete") init();
  else window.onload = init;
})();
