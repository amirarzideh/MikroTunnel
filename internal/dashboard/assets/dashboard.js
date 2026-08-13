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
      tunnels.items = Array.isArray(tunnels.items) ? tunnels.items : [];
      operations.items = Array.isArray(operations.items) ? operations.items : [];
      connection.textContent = 'Connected'; connection.className = 'connection connected';
      $('agent-name').textContent = system.hostname || 'Ubuntu agent'; $('agent-kernel').textContent = system.kernel || 'Linux';
      $('tunnel-count').textContent = tunnels.items.length;
      $('healthy-count').textContent = tunnels.items.filter(t => t.actual_state === 'up').length;
      $('pending-count').textContent = operations.items.filter(o => o.status === 'queued' || o.status === 'running').length;
      $('tunnels').innerHTML = tunnels.items.length ? tunnels.items.map(t => `<article class="tunnel"><div><div class="tunnel-name">${safe(t.name)}</div><span class="tunnel-detail">${safe(t.local_endpoint)} to ${safe(t.remote_endpoint)} · ${safe(t.address)}${t.last_error ? '<br><span class="repair-note">Repair: ' + safe(t.last_error) + (t.retry_at ? ' · retry scheduled' : '') + '</span>' : ''}</span></div><div class="tunnel-actions">${badge(t.actual_state)} ${badge(t.desired_state)}<button class="action" data-id="${safe(t.id)}" data-action="edit">Edit</button><button class="action" data-id="${safe(t.id)}" data-action="${t.desired_state === 'enabled' ? 'disable' : 'enable'}">${t.desired_state === 'enabled' ? 'Disable' : 'Enable'}</button><button class="action danger" data-id="${safe(t.id)}" data-action="delete">Remove</button></div></article>`).join('') : '<p class="empty">No tunnels have been configured.</p>';
      $('operations').innerHTML = operations.items.length ? operations.items.slice(0, 8).map(o => `<article class="operation"><div><strong>${safe(o.action.replaceAll('_',' '))}</strong><small>${safe(o.message || 'Processing')}</small></div>${badge(o.status)}</article>`).join('') : '<p class="empty">No operations yet.</p>';
    } catch (error) { connection.textContent = 'Connection failed'; connection.className = 'connection'; if (String(error.message).includes('unauthorized')) { sessionStorage.removeItem('mikrotunnel-api-key'); state.key = ''; connectDialog.showModal(); } }
  }
  $('connect-form').addEventListener('submit', event => { if (event.submitter?.value !== 'connect') return; event.preventDefault(); state.key = $('api-key').value.trim(); sessionStorage.setItem('mikrotunnel-api-key', state.key); connectDialog.close(); load(); });
  $('new-tunnel').addEventListener('click', () => { if (!state.key) return connectDialog.showModal(); const form = $('tunnel-form'); form.reset(); delete form.dataset.editId; $('tunnel-mode').textContent = 'NEW GRE TUNNEL'; $('tunnel-title').textContent = 'Desired configuration'; $('tunnel-submit').textContent = 'Queue tunnel'; tunnelDialog.showModal(); });
  $('tunnel-form').addEventListener('submit', async event => { if (event.submitter?.value !== 'create') return; event.preventDefault(); const form = new FormData(event.currentTarget); const body = Object.fromEntries(form.entries()); body.type = 'gre'; body.mtu = Number(body.mtu); body.ttl = Number(body.ttl); const editId = event.currentTarget.dataset.editId; $('form-error').textContent = ''; try { await api(editId ? '/tunnels/' + encodeURIComponent(editId) : '/tunnels', { method: editId ? 'PUT' : 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) }); tunnelDialog.close(); event.currentTarget.reset(); load(); } catch (error) { $('form-error').textContent = error.message; } });
  $('tunnels').addEventListener('click', async event => { const button = event.target.closest('button[data-action]'); if (!button) return; const { id, action } = button.dataset; if (action === 'edit') { try { const tunnel = await api('/tunnels/' + encodeURIComponent(id)); const form = $('tunnel-form'); for (const key of ['name','address','local_endpoint','remote_endpoint','mtu','ttl','description']) form.elements[key].value = tunnel[key] || ''; form.dataset.editId = id; $('tunnel-mode').textContent = 'EDIT GRE TUNNEL'; $('tunnel-title').textContent = 'Update desired configuration'; $('tunnel-submit').textContent = 'Save changes'; tunnelDialog.showModal(); } catch (error) { alert(error.message); } return; } if (action === 'delete' && !confirm('Queue safe removal of this tunnel?')) return; button.disabled = true; try { await api('/tunnels/' + encodeURIComponent(id) + (action === 'delete' ? '' : '/' + action), { method: action === 'delete' ? 'DELETE' : 'POST' }); await load(); } catch (error) { alert(error.message); button.disabled = false; } });
  $('refresh').addEventListener('click', load);
  connection.addEventListener('click', () => connectDialog.showModal());
  load(); setInterval(load, 10000);
})();
