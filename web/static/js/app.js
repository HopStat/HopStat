// LG Looking Glass — Frontend Application
(function () {
  'use strict';

  const API = '/api/v1';
  let queryStore = {};

  // ── Utility ──────────────────────────────────────────────
  function $(sel) { return document.querySelector(sel); }
  function $$(sel) { return document.querySelectorAll(sel); }

  function formatRTT(ms) {
    if (ms === 0 || ms == null) return '—';
    return ms.toFixed(2) + ' ms';
  }

  function formatLoss(pct) {
    if (pct == null) return '—';
    return pct.toFixed(1) + '%';
  }

  function escapeHTML(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  // ── API Helpers ──────────────────────────────────────────
  async function apiFetch(path, opts) {
    const res = await fetch(API + path, {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data.data;
  }

  // ── Node Loading ─────────────────────────────────────────
  async function loadNodes() {
    try {
      const nodes = await apiFetch('/nodes');
      const sel = $('#node-select');
      if (!sel) return;
      sel.innerHTML = '<option value="">— Select Node —</option>';
      (nodes || []).forEach(function (n) {
        const opt = document.createElement('option');
        opt.value = n.id;
        opt.textContent = n.name + (n.description ? ' — ' + n.description : '');
        sel.appendChild(opt);
      });
    } catch (e) {
      console.error('Failed to load nodes:', e);
    }
  }

  // ── Query Submission ─────────────────────────────────────
  async function submitQuery() {
    const nodeSelect = $('#node-select');
    const commandSelect = $('#command-select');
    const targetInput = $('#target-input');
    const resultArea = $('#result-area');
    const errorArea = $('#error-area');
    const submitBtn = $('#submit-btn');

    const nodeId = parseInt(nodeSelect.value);
    const command = commandSelect.value;
    const target = targetInput.value.trim();

    if (!nodeId || !command || !target) return;

    resultArea.innerHTML = '<div class="loading">Running query...</div>';
    errorArea.innerHTML = '';

    submitBtn.disabled = true;
    submitBtn.textContent = 'Running...';

    const body = {
      node_id: nodeId,
      command: command,
      target: target,
      options: {},
    };

    // Command-specific options
    if (command === 'ping') {
      body.options.ping_count = parseInt($('#opt-ping-count')?.value) || 5;
    } else if (command === 'traceroute') {
      body.options.max_hops = parseInt($('#opt-max-hops')?.value) || 30;
    } else if (command === 'mtr') {
      body.options.mtr_cycles = parseInt($('#opt-mtr-cycles')?.value) || 10;
    }

    try {
      const resp = await fetch(API + '/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const data = await resp.json();
      if (data.error) {
        errorArea.innerHTML = '<div class="error">' + escapeHTML(data.error.message || data.error.code || 'Unknown error') + '</div>';
        resultArea.innerHTML = '';
        return;
      }

      const queryId = data.data?.query_id;
      if (queryId) {
        pollResult(queryId);
      }
    } catch (e) {
      errorArea.innerHTML = '<div class="error">' + escapeHTML(e.message) + '</div>';
      resultArea.innerHTML = '';
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = 'Query';
    }
  }

  // ── Result Polling ───────────────────────────────────────
  async function pollResult(queryId) {
    const resultArea = $('#result-area');
    const maxPolls = 60;

    for (let i = 0; i < maxPolls; i++) {
      await new Promise(function (r) { setTimeout(r, 1000); });

      try {
        const res = await fetch(API + '/query/' + queryId);
        const data = await res.json();
        const result = data.data;

        if (!result) continue;

        renderResult(result, resultArea);

        if (result.status === 'done' || result.status === 'error') {
          return;
        }
      } catch (e) {
        console.error('Poll error:', e);
      }
    }

    resultArea.innerHTML = '<div class="error">Query timed out</div>';
  }

  // ── Result Rendering ─────────────────────────────────────
  function renderResult(result, container) {
    if (result.status === 'error') {
      container.innerHTML =
        '<div class="error">' +
        '<strong>Error:</strong> ' + escapeHTML(result.error_msg || 'Unknown error') +
        (result.error_code ? ' <code>' + escapeHTML(result.error_code) + '</code>' : '') +
        '</div>';
      return;
    }

    const parsed = result.parsed;
    if (!parsed) {
      container.innerHTML = result.raw ? '<pre>' + escapeHTML(result.raw) + '</pre>' : '<p>No result</p>';
      return;
    }

    const command = result.command || '';
    let html = '';

    // Duration badge
    if (result.duration_ms) {
      html += '<div class="duration">' + result.duration_ms + ' ms</div>';
    }

    if (parsed.packets_sent !== undefined) {
      html += renderPingResult(parsed);
    } else if (parsed.hops && parsed.hops.length && parsed.hops[0].rtt) {
      html += renderTracerouteResult(parsed);
    } else if (parsed.hops && parsed.hops.length && parsed.hops[0].loss !== undefined) {
      html += renderMTRResult(parsed);
    } else if (parsed.routes) {
      html += renderBGPResult(parsed, result);
    } else if (parsed.prefixes) {
      html += renderASPathResult(parsed);
    } else {
      html += '<pre>' + escapeHTML(result.raw || JSON.stringify(parsed, null, 2)) + '</pre>';
    }

    // Raw output toggle
    if (result.raw) {
      html += '<details><summary>Raw Output</summary><pre>' + escapeHTML(result.raw) + '</pre></details>';
    }

    container.innerHTML = html;
  }

  function renderPingResult(p) {
    return (
      '<div class="result-grid">' +
      '<div class="stat"><label>Sent</label><span>' + p.packets_sent + '</span></div>' +
      '<div class="stat"><label>Received</label><span>' + p.packets_recv + '</span></div>' +
      '<div class="stat"><label>Loss</label><span>' + formatLoss(p.packet_loss) + '</span></div>' +
      '<div class="stat"><label>Min RTT</label><span>' + formatRTT(p.min_rtt) + '</span></div>' +
      '<div class="stat"><label>Avg RTT</label><span>' + formatRTT(p.avg_rtt) + '</span></div>' +
      '<div class="stat"><label>Max RTT</label><span>' + formatRTT(p.max_rtt) + '</span></div>' +
      '</div>'
    );
  }

  function renderTracerouteResult(p) {
    let html = '<table class="hop-table"><tr><th>#</th><th>IP</th><th>Host</th><th>RTT</th><th>AS</th></tr>';
    (p.hops || []).forEach(function (hop) {
      const asInfo = hop.as_info || {};
      const asBadge = asInfo.asn ? '<span class="as-badge">AS' + asInfo.asn + '</span> ' + escapeHTML(asInfo.org_name || '') : '';
      const rtt = (hop.rtt || []).map(function (r) { return r.toFixed(2) + ' ms'; }).join(', ');
      html += '<tr>' +
        '<td>' + hop.number + '</td>' +
        '<td>' + escapeHTML(hop.ip || '*') + '</td>' +
        '<td>' + escapeHTML(hop.host || '') + '</td>' +
        '<td>' + rtt + '</td>' +
        '<td>' + asBadge + '</td>' +
        '</tr>';
    });
    html += '</table>';
    return html;
  }

  function renderMTRResult(p) {
    let html = '<table class="hop-table"><tr><th>#</th><th>Host</th><th>Loss%</th><th>Sent</th><th>Recv</th><th>Avg</th><th>Best</th><th>Worst</th><th>AS</th></tr>';
    (p.hops || []).forEach(function (hop) {
      const asInfo = hop.as_info || {};
      const asBadge = asInfo.asn ? '<span class="as-badge">AS' + asInfo.asn + '</span>' : '';
      html += '<tr>' +
        '<td>' + hop.number + '</td>' +
        '<td>' + escapeHTML(hop.host || '*') + '</td>' +
        '<td>' + formatLoss(hop.loss) + '</td>' +
        '<td>' + hop.sent + '</td>' +
        '<td>' + hop.recv + '</td>' +
        '<td>' + formatRTT(hop.avg) + '</td>' +
        '<td>' + formatRTT(hop.best) + '</td>' +
        '<td>' + formatRTT(hop.worst) + '</td>' +
        '<td>' + asBadge + '</td>' +
        '</tr>';
    });
    html += '</table>';
    return html;
  }

  function renderBGPResult(p, result) {
    let html = '';
    if (result && result.as_path_enriched && result.as_path_enriched.length > 0) {
      html += '<div class="as-path"><strong>AS Path:</strong> ';
      result.as_path_enriched.forEach(function (as) {
        html += '<span class="as-badge" title="' + escapeHTML(as.org_name || '') + '">AS' + as.asn + '</span> ';
      });
      html += '</div>';
    }

    if (p.routes && p.routes.length > 0) {
      html += '<table class="hop-table"><tr><th>Prefix</th><th>Next Hop</th><th>AS Path</th><th>Origin</th><th>Communities</th></tr>';
      p.routes.forEach(function (route) {
        const asPath = (route.as_path || []).map(function (a) { return 'AS' + a; }).join(' → ');
        html += '<tr>' +
          '<td><code>' + escapeHTML(route.prefix) + '</code></td>' +
          '<td>' + escapeHTML(route.next_hop || '') + '</td>' +
          '<td>' + (asPath || '—') + '</td>' +
          '<td>' + escapeHTML(route.origin || '') + '</td>' +
          '<td>' + (route.communities || []).map(function (c) { return '<code>' + escapeHTML(c) + '</code>'; }).join(' ') + '</td>' +
          '</tr>';
      });
      html += '</table>';
    } else {
      html += '<p>No routes found</p>';
    }
    return html;
  }

  function renderASPathResult(p) {
    let html = '<h4>AS' + p.asn + '</h4>';
    if (p.prefixes && p.prefixes.length > 0) {
      html += '<table class="hop-table"><tr><th>Prefix</th><th>AS Path</th></tr>';
      p.prefixes.forEach(function (entry) {
        const asPath = (entry.as_path || []).map(function (a) { return 'AS' + a; }).join(' → ');
        html += '<tr><td><code>' + escapeHTML(entry.prefix) + '</code></td><td>' + (asPath || '—') + '</td></tr>';
      });
      html += '</table>';
    } else {
      html += '<p>No prefixes found for AS' + p.asn + '</p>';
    }
    return html;
  }

  // ── SSE Streaming ────────────────────────────────────────
  function streamResult(queryId) {
    const resultArea = $('#result-area');
    const evtSource = new EventSource(API + '/query/' + queryId + '/stream');

    evtSource.addEventListener('result', function (e) {
      try {
        const data = JSON.parse(e.data);
        renderResult(data, resultArea);
        if (data.status === 'done' || data.status === 'error') {
          evtSource.close();
        }
      } catch (err) {
        console.error('SSE parse error:', err);
      }
    });

    evtSource.addEventListener('progress', function (e) {
      try {
        const data = JSON.parse(e.data);
        if (data.raw) {
          resultArea.innerHTML = '<pre class="streaming">' + escapeHTML(data.raw) + '</pre>';
        }
      } catch (err) { /* ignore */ }
    });

    evtSource.onerror = function () {
      evtSource.close();
      // Fall back to polling
      pollResult(queryId);
    };
  }

  // ── Command Options Toggle ───────────────────────────────
  function setupCommandOptions() {
    const cmdSelect = $('#command-select');
    if (!cmdSelect) return;

    cmdSelect.addEventListener('change', function () {
      const cmd = cmdSelect.value;
      $$('.cmd-options').forEach(function (el) { el.style.display = 'none'; });

      if (cmd === 'ping') {
        const el = $('#opt-ping'); if (el) el.style.display = 'block';
      } else if (cmd === 'traceroute') {
        const el = $('#opt-traceroute'); if (el) el.style.display = 'block';
      } else if (cmd === 'mtr') {
        const el = $('#opt-mtr'); if (el) el.style.display = 'block';
      }
    });
  }

  // ── Target Validation ────────────────────────────────────
  function validateTarget() {
    const cmd = ($('#command-select') || {}).value;
    const target = ($('#target-input') || {}).value.trim();

    if (!target) return true; // handled by required check

    if (cmd === 'bgp_route') {
      // Accept IP or CIDR
      return /^[\d.:a-fA-F\/]+$/.test(target);
    }
    if (cmd === 'as_path') {
      // Accept numeric ASN
      return /^\d+$/.test(target);
    }
    // ping, traceroute, mtr — accept IP or hostname
    return true;
  }

  // ── Form Submit ──────────────────────────────────────────
  function setupForm() {
    const form = $('#query-form');
    if (!form) return;

    form.addEventListener('submit', function (e) {
      e.preventDefault();
      submitQuery();
    });

    const submitBtn = $('#submit-btn');
    if (submitBtn) {
      submitBtn.addEventListener('click', function (e) {
        e.preventDefault();
        submitQuery();
      });
    }
  }

  // ── Init ─────────────────────────────────────────────────
  function init() {
    loadNodes();
    setupCommandOptions();
    setupForm();
  }

  // Auto-init on DOM ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
