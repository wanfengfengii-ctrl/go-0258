// Browser console: fetches live backend state from the Go API and drives a
// tank-batch inspection from build through finalization.
const healthEl = document.getElementById('health');
const tbody = document.querySelector('#tasks tbody');
const createMsg = document.getElementById('create-msg');
const detailSection = document.getElementById('detail-section');
const detailEl = document.getElementById('detail');

async function api(path, opts) {
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data.error || 'error') + (data.reasons ? ': ' + data.reasons.join(', ') : ''));
  }
  return data;
}

async function loadHealth() {
  try {
    const data = await api('/api/health');
    healthEl.textContent = '后端状态：' + data.status;
  } catch (err) {
    healthEl.textContent = '无法连接后端：' + err.message;
  }
}

async function loadTasks() {
  try {
    const data = await api('/api/tasks');
    tbody.innerHTML = '';
    for (const t of (data.tasks ?? [])) {
      const tr = document.createElement('tr');
      tr.innerHTML =
        `<td>${escapeHtml(t.id)}</td><td>${escapeHtml(t.tankBatch)}</td><td>${escapeHtml(t.status)}</td>` +
        `<td>${t.generation}</td><td>${escapeHtml(t.finalType || '—')}</td>` +
        `<td><button type="button" data-id="${escapeHtml(t.id)}">查看</button></td>`;
      tbody.appendChild(tr);
    }
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6">加载失败：${escapeHtml(err.message)}</td></tr>`;
  }
}

async function loadDetail(id) {
  try {
    const data = await api('/api/tasks/' + encodeURIComponent(id) + '/report');
    detailSection.classList.remove('hidden');
    detailEl.innerHTML = renderReport(data);
  } catch (err) {
    detailEl.innerHTML = `<p class="error">${escapeHtml(err.message)}</p>`;
    detailSection.classList.remove('hidden');
  }
}

function renderReport(r) {
  const readings = (r.readings ?? []).map((x) =>
    `<tr><td>${escapeHtml(x.type)}</td><td>${escapeHtml(x.blindCode)}</td><td>${escapeHtml(x.value)}</td><td>${x.pass ? '通过' : '不通过'}</td></tr>`
  ).join('');
  const reviews = (r.reviews ?? []).map((x) =>
    `<li>${escapeHtml(x.reviewer)}：${escapeHtml(x.conclusion)}</li>`
  ).join('');
  const rejudgements = (r.rejudgements ?? []).map((x) =>
    `<li>${escapeHtml(x.reason)} (代次 ${x.generation})</li>`
  ).join('');
  const cold = r.coldChain || {};
  return `
    <p><strong>任务</strong> ${escapeHtml(r.taskId)} · <strong>批号</strong> ${escapeHtml(r.tankBatch)}</p>
    <p><strong>状态</strong> ${escapeHtml(r.status)} · <strong>代次</strong> ${r.generation} · <strong>结论</strong> ${escapeHtml(r.decision || '未决')}</p>
    <h3>冷链覆盖</h3>
    <p>覆盖 ${cold.coveredCount}/${cold.expectedCount} · 连续超温 ${cold.consecutiveOverSeconds}s · ${cold.complete ? '完整' : '不完整'}</p>
    <h3>检测读数</h3>
    <table><thead><tr><th>类型</th><th>盲码</th><th>值</th><th>结论</th></tr></thead><tbody>${readings || '<tr><td colspan="4">无</td></tr>'}</tbody></table>
    <h3>独立复核</h3>
    <ul>${reviews || '<li>无</li>'}</ul>
    <h3>复判</h3>
    <ul>${rejudgements || '<li>无</li>'}</ul>
  `;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

document.getElementById('refresh').addEventListener('click', () => {
  loadHealth();
  loadTasks();
});

tbody.addEventListener('click', (e) => {
  const btn = e.target.closest('button[data-id]');
  if (btn) loadDetail(btn.dataset.id);
});

document.getElementById('create-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  createMsg.textContent = '';
  const fd = new FormData(e.target);
  const split = (s) => s.split(',').map((x) => x.trim()).filter(Boolean);
  const body = {
    taskId: fd.get('taskId'),
    farmId: fd.get('farmId'),
    tankBatch: fd.get('tankBatch'),
    compartments: split(fd.get('compartments')),
    seals: split(fd.get('seals')),
    recorderModel: fd.get('recorderModel'),
    ruleVersion: fd.get('ruleVersion'),
    reviewers: split(fd.get('reviewers')),
  };
  try {
    await api('/api/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    createMsg.textContent = '建检成功';
    createMsg.className = 'msg ok';
    loadTasks();
  } catch (err) {
    createMsg.textContent = err.message;
    createMsg.className = 'msg error';
  }
});

loadHealth();
loadTasks();
