(() => {
  const state = { key: sessionStorage.getItem('mikrotunnel-api-key') || '' };
  const $ = id => document.getElementById(id);
  const connection = $('connection');
  const connectDialog = $('connect-dialog');
  const tunnelDialog = $('tunnel-dialog');
  const api = async (path, options = {}) => {
    const response = await fetch('/api/v1' + path, { ...options, headers: { ...options.headers, Authorization: 'Bearer ' + state.key } });
    if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error || 'Request failed'); }
    return response.json();
  };
  const badge = value => `<span class="badge ${value}">${value}</span>`;
  const safe = value => String(value || '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[char]));
  async function load() {
    if (!state.key) { connectDialog.showModal(); return; }
    try {
      const [system, tunnels, operations] = await Promise.all([api('/system'), api('/tunnels'), api('/operations')]);
      connection.textContent = 'Connected'; connection.className = 'connection connected';
      $('agent-name').textContent = system.hostname || 'Ubuntu agent'; $('agent-kernel').textContent = system.kernel || 'Linux';
      $('tunnel-count').textContent = tunnels.items.length;
      $('healthy-count').textContent = tunnels.items.filter(t => t.actual_state === 'up').length;
      $('pending-count').textContent = operations.items.filter(o => o.status === 'queued' || o.status === 'running').length;
      $('tunnels').innerHTML = tunnels.items.length ? tunnels.items.map(t => `<article class="tunnel"><div><div class="tunnel-name">${safe(t.name)}</div><span class="tunnel-detail">${safe(t.local_endpoint)} → ${safe(t.remote_endpoint)} · ${safe(t.address)}</span></div><div>${badge(t.actual_state)} ${badge(t.desired_state)}</div></article>`).join('') : '<p class="empty">No tunnels have been configured.</p>';
      $('operations').innerHTML = operations.items.length ? operations.items.slice(0, 8).map(o => `<article class="operation"><div><strong>${safe(o.action.replaceAll('_',' '))}</strong><small>${safe(o.message || 'Processing')}</small></div>${badge(o.status)}</article>`).join('') : '<p class="empty">No operations yet.</p>';
    } catch (error) { connection.textContent = 'Connection failed'; connection.className = 'connection'; if (String(error.message).includes('unauthorized')) { sessionStorage.removeItem('mikrotunnel-api-key'); state.key = ''; connectDialog.showModal(); } }
  }
  $('connect-form').addEventListener('submit', event => { if (event.submitter?.value !== 'connect') return; event.preventDefault(); state.key = $('api-key').value.trim(); sessionStorage.setItem('mikrotunnel-api-key', state.key); connectDialog.close(); load(); });
  $('new-tunnel').addEventListener('click', () => { if (!state.key) connectDialog.showModal(); else tunnelDialog.showModal(); });
  $('tunnel-form').addEventListener('submit', async event => { if (event.submitter?.value !== 'create') return; event.preventDefault(); const form = new FormData(event.currentTarget); const body = Object.fromEntries(form.entries()); body.type = 'gre'; body.mtu = Number(body.mtu); body.ttl = Number(body.ttl); $('form-error').textContent = ''; try { await api('/tunnels', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) }); tunnelDialog.close(); event.currentTarget.reset(); load(); } catch (error) { $('form-error').textContent = error.message; } });
  $('refresh').addEventListener('click', load);
  connection.addEventListener('click', () => connectDialog.showModal());
  load(); setInterval(load, 10000);
})();
