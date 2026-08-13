(() => {
  const $ = id => document.getElementById(id);
  const api = async (path, options = {}) => {
    const response = await fetch('/api/v1' + path, { ...options, credentials: 'same-origin', headers: { ...options.headers } });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(body.error || 'Request failed');
    return body;
  };
  const safe = value => String(value || '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[c]));
  const badge = value => `<span class="badge ${safe(value)}">${safe(value)}</span>`;
  let loading = false;
  async function load() {
    if (loading) return; loading = true;
    try {
	  const [{items=[]}, settings, discovered] = await Promise.all([api('/tunnels'), api('/network/settings'), api('/interfaces')]);
	  $('ipv4-forward').checked = !!settings.ipv4_forward;
      $('connection').textContent = 'Live'; $('connection').className = 'connection connected';
	  const known = new Set(items.map(t => t.name)); const unmanaged = (discovered.items||[]).filter(t => !known.has(t.name));
	  const managed = items.map(t => `<article class="tunnel-card"><div class="tunnel-top"><div><strong>${safe(t.name)}</strong><span>${safe(t.local_endpoint)} → ${safe(t.remote_endpoint)} · ${safe(t.address)}${t.peer_address ? ' · peer '+safe(t.peer_address) : ''}</span></div>${badge(t.actual_state)}</div><div class="tunnel-meta">${badge(t.desired_state)} ${t.peer_address ? '<span class="badge" id="peer-'+safe(t.id)+'">peer unchecked</span>' : ''}${t.masquerade ? '<span class="badge enabled">NAT enabled</span>' : ''}${t.last_error ? `<span class="error">${safe(t.last_error)}</span>` : ''}</div><div class="tunnel-actions"><button data-action="probe" data-id="${safe(t.id)}">Ping peer</button><button data-action="internet" data-id="${safe(t.id)}">Ping internet</button><button data-action="edit" data-id="${safe(t.id)}">Edit</button><button data-action="${t.desired_state === 'enabled' ? 'disable' : 'enable'}" data-id="${safe(t.id)}">${t.desired_state === 'enabled' ? 'Disable' : 'Enable'}</button><button class="danger" data-action="delete" data-id="${safe(t.id)}">Remove</button></div><pre id="probe-${safe(t.id)}" class="probe"></pre></article>`).join('');
	  const found = unmanaged.map(t => `<article class="tunnel-card"><div class="tunnel-top"><div><strong>${safe(t.name)}</strong><span>${safe(t.local_endpoint)} → ${safe(t.remote_endpoint)} · ${safe(t.address||'no IPv4 address')}</span></div><span class="badge">discovered</span></div><div class="tunnel-meta"><span class="error">Not in MikroTunnel state</span></div><div class="tunnel-actions">${String(t.alias||'').startsWith('mikrotunnel:') ? `<button data-action="import-discovered" data-name="${safe(t.name)}">Import & manage</button>` : ''}<button class="danger" data-action="remove-discovered" data-name="${safe(t.name)}">Remove interface</button></div></article>`).join('');
	  $('tunnels').innerHTML = managed + found || '<p class="empty">No GRE interfaces found.</p>';
    } catch (error) { $('connection').textContent = 'Offline'; $('connection').className = 'connection'; }
    finally { loading = false; }
  }
  $('refresh').onclick = load;
	$('ipv4-forward').onchange = async event => { const control=event.currentTarget; control.disabled=true; try { await api('/network/ipv4-forward',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({enabled:control.checked})}); } catch(error) { alert(error.message); control.checked=!control.checked; } finally { control.disabled=false; } };
  $('new-tunnel').onclick = () => { const f=$('tunnel-form'); f.reset(); delete f.dataset.id; $('tunnel-title').textContent='Add tunnel'; $('tunnel-submit').textContent='Create tunnel'; $('tunnel-dialog').showModal(); };
  $('tunnels').onclick = async event => {
    const button=event.target.closest('button[data-action]'); if (!button) return;
	const {id,action,name}=button.dataset; button.disabled=true;
    try {
	  if (action==='remove-discovered') { if (!confirm('Remove the discovered GRE interface '+name+'?')) return; await api('/interfaces/'+encodeURIComponent(name),{method:'DELETE'}); await load(); return; }
	  if (action==='import-discovered') { await api('/interfaces/'+encodeURIComponent(name)+'/import',{method:'POST'}); await load(); return; }
	  if (action==='probe' || action==='internet') { const suffix=action==='internet'?'?target=1.1.1.1':''; const r=await api('/tunnels/'+encodeURIComponent(id)+'/probe'+suffix,{method:'POST'}); const status=$(action==='probe'?'peer-'+id:'probe-'+id); if(status) { status.textContent=r.reachable ? (action==='probe'?'peer reachable':'internet reachable') : (action==='probe'?'peer unreachable':'internet unreachable'); status.className=r.reachable?'badge up':'badge error'; } $('probe-'+id).textContent=r.output; return; }
	  if (action==='edit') { const t=await api('/tunnels/'+encodeURIComponent(id)); const f=$('tunnel-form'); ['name','address','peer_address','local_endpoint','remote_endpoint','mtu','ttl','description'].forEach(k=>f.elements[k].value=t[k]||''); f.elements.masquerade.checked=!!t.masquerade; f.dataset.id=id; $('tunnel-title').textContent='Edit tunnel'; $('tunnel-submit').textContent='Save changes'; $('tunnel-dialog').showModal(); return; }
      if (action==='delete' && !confirm('Remove this managed tunnel?')) return;
      await api('/tunnels/'+encodeURIComponent(id)+(action==='delete'?'':'/'+action),{method:action==='delete'?'DELETE':'POST'}); await load();
    } catch(error) { alert(error.message); } finally { button.disabled=false; }
  };
  $('tunnel-form').onsubmit = async event => { event.preventDefault(); const f=event.currentTarget; const data=Object.fromEntries(new FormData(f)); data.type='gre'; data.mtu=Number(data.mtu); data.ttl=Number(data.ttl); data.masquerade=f.elements.masquerade.checked; try { await api(f.dataset.id?'/tunnels/'+f.dataset.id:'/tunnels',{method:f.dataset.id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)}); $('tunnel-dialog').close(); load(); } catch(error) { $('form-error').textContent=error.message; } };
  load(); setInterval(load, 5000);
})();
