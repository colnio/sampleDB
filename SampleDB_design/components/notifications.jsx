// Notifications — shared state across all directions
// Admins create notes that appear as a closable top banner for everyone.

(function(){
  const KEY_DISMISS = 'sampledb_dismissed_notifs';

  // module-level store + simple subscription
  let id = 4;
  let _notifs = [
    { id: 1, level: 'info',     message: 'Maintenance window scheduled Sat Apr 26, 22:00 — all booking writes will pause for ~30 min.', author: 'msmith', createdAt: '2026-04-19 09:12' },
    { id: 2, level: 'warning',  message: 'Raman Spectrometer laser power calibration drifted — re-run baselines until further notice.', author: 'msmith', createdAt: '2026-04-18 16:40' },
  ];
  const listeners = new Set();
  const emit = () => listeners.forEach(fn => fn());

  function getDismissed() {
    try { return new Set(JSON.parse(localStorage.getItem(KEY_DISMISS) || '[]')); }
    catch { return new Set(); }
  }
  function setDismissed(set) {
    try { localStorage.setItem(KEY_DISMISS, JSON.stringify([...set])); } catch {}
  }

  window.NotifStore = {
    list: () => _notifs.slice(),
    add: (n) => { _notifs = [{ id: id++, level: 'info', author: 'msmith', createdAt: new Date().toISOString().slice(0,16).replace('T',' '), ...n }, ..._notifs]; emit(); },
    remove: (nid) => { _notifs = _notifs.filter(n => n.id !== nid); emit(); },
    dismiss: (nid) => { const d = getDismissed(); d.add(nid); setDismissed(d); emit(); },
    undismiss: (nid) => { const d = getDismissed(); d.delete(nid); setDismissed(d); emit(); },
    isDismissed: (nid) => getDismissed().has(nid),
    subscribe: (fn) => { listeners.add(fn); return () => listeners.delete(fn); },
  };
})();

function useNotifs() {
  const [, force] = React.useReducer(x => x+1, 0);
  React.useEffect(() => window.NotifStore.subscribe(force), []);
  return {
    all: window.NotifStore.list(),
    visible: window.NotifStore.list().filter(n => !window.NotifStore.isDismissed(n.id)),
    add: window.NotifStore.add,
    remove: window.NotifStore.remove,
    dismiss: window.NotifStore.dismiss,
  };
}

// Direction A banner — soft card with colored left bar
function A_NotifBanner() {
  const { visible, dismiss } = useNotifs();
  if (!visible.length) return null;
  const palette = { info: { bg:'rgba(37,99,235,0.08)', bd:'rgba(37,99,235,0.25)', accent:'#2563eb', label:'Info' },
                    warning: { bg:'rgba(245,158,11,0.10)', bd:'rgba(245,158,11,0.30)', accent:'#b45309', label:'Warning' },
                    critical: { bg:'rgba(220,38,38,0.10)', bd:'rgba(220,38,38,0.30)', accent:'#dc2626', label:'Critical' } };
  return (
    <div className="a-notif-stack">
      {visible.map(n => {
        const p = palette[n.level] || palette.info;
        return (
          <div key={n.id} className="a-notif" style={{ background: p.bg, borderColor: p.bd }}>
            <div className="a-notif__bar" style={{ background: p.accent }}/>
            <div className="a-notif__body">
              <div className="a-notif__head">
                <span className="a-notif__level" style={{ color: p.accent, borderColor: p.bd }}>{p.label}</span>
                <span className="a-notif__msg">{n.message}</span>
              </div>
              <div className="a-notif__meta">posted by {n.author} · {n.createdAt}</div>
            </div>
            <button className="a-notif__close" onClick={() => dismiss(n.id)} aria-label="Dismiss"><Ic.Close/></button>
          </div>
        );
      })}
    </div>
  );
}

// Direction B banner — single dense line per notif
function B_NotifBanner() {
  const { visible, dismiss } = useNotifs();
  if (!visible.length) return null;
  return (
    <div className="b-notif-stack">
      {visible.map(n => (
        <div key={n.id} className={'b-notif b-notif--' + n.level}>
          <span className="b-notif__tag">{n.level.toUpperCase()}</span>
          <span className="b-notif__msg">{n.message}</span>
          <span className="b-notif__meta">{n.author} · {n.createdAt}</span>
          <button className="b-notif__close" onClick={() => dismiss(n.id)} aria-label="Dismiss"><Ic.Close style={{width:11,height:11}}/></button>
        </div>
      ))}
    </div>
  );
}

// Direction C banner — editorial, framed in ink rules
function C_NotifBanner() {
  const { visible, dismiss } = useNotifs();
  if (!visible.length) return null;
  return (
    <div className="c-notif-stack">
      {visible.map(n => (
        <div key={n.id} className={'c-notif c-notif--' + n.level}>
          <div className="c-notif__rule"/>
          <div className="c-notif__body">
            <div className="c-notif__eyebrow">{n.level === 'info' ? 'Notice' : n.level === 'warning' ? 'Advisory' : 'Critical'} · {n.createdAt}</div>
            <div className="c-notif__msg">{n.message}</div>
          </div>
          <button className="c-notif__close" onClick={() => dismiss(n.id)} aria-label="Dismiss"><Ic.Close style={{width:14,height:14}}/></button>
        </div>
      ))}
    </div>
  );
}

// Shared admin manager — composer + list (caller wraps in their own card chrome)
function NotifAdminPanel({ direction }) {
  const { all, add, remove } = useNotifs();
  const [msg, setMsg] = React.useState('');
  const [level, setLevel] = React.useState('info');
  const submit = () => { if (!msg.trim()) return; add({ message: msg.trim(), level }); setMsg(''); setLevel('info'); };
  const cls = direction === 'B' ? 'dirB' : direction === 'C' ? 'dirC' : 'dirA';
  const inputStyle = direction === 'B' ? { fontFamily: 'JetBrains Mono, monospace', fontSize: 12.5 } :
                      direction === 'C' ? { fontFamily: 'Inter, sans-serif' } : {};
  const palette = { info:'#2563eb', warning:'#b45309', critical:'#dc2626' };
  return (
    <div className="notif-admin">
      <div className="notif-admin__composer">
        <div className="field">
          <label>Message</label>
          <textarea value={msg} onChange={e=>setMsg(e.target.value)} placeholder="Brief note for the lab — visible at the top of every page until dismissed." style={{minHeight: 64, ...inputStyle}}/>
        </div>
        <div className="notif-admin__row">
          <div className="field">
            <label>Severity</label>
            <select value={level} onChange={e=>setLevel(e.target.value)} style={inputStyle}>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </select>
          </div>
          <button className="btn btn--primary" onClick={submit} disabled={!msg.trim()}><Ic.Plus/>Post notification</button>
        </div>
      </div>
      <div className="notif-admin__list">
        {all.length === 0 && <div className="notif-admin__empty">No notifications posted.</div>}
        {all.map(n => (
          <div key={n.id} className="notif-admin__item">
            <span className="notif-admin__dot" style={{ background: palette[n.level] }}/>
            <div className="notif-admin__item-body">
              <div className="notif-admin__item-msg">{n.message}</div>
              <div className="notif-admin__item-meta">{n.level} · {n.author} · {n.createdAt}</div>
            </div>
            <button className="btn btn--ghost btn--sm" onClick={() => remove(n.id)}><Ic.Trash/></button>
          </div>
        ))}
      </div>
    </div>
  );
}

Object.assign(window, { useNotifs, A_NotifBanner, B_NotifBanner, C_NotifBanner, NotifAdminPanel });
