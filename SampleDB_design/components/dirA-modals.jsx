// Direction A modals - reusable Modal + 4 specific dialogs
function A_Modal({ open, onClose, title, subtitle, children, footer, width = 520 }) {
  React.useEffect(() => {
    if (!open) return;
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    document.body.style.overflow = 'hidden';
    return () => { window.removeEventListener('keydown', onKey); document.body.style.overflow = ''; };
  }, [open]);
  if (!open) return null;
  return (
    <div className="dirA-modal__backdrop" onClick={onClose}>
      <div className="dirA-modal" style={{maxWidth: width}} onClick={e => e.stopPropagation()}>
        <div className="dirA-modal__head">
          <div>
            <h3>{title}</h3>
            {subtitle && <div className="dirA-modal__sub">{subtitle}</div>}
          </div>
          <button className="dirA-modal__close" onClick={onClose} aria-label="Close">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none"><path d="M6 6l12 12M6 18L18 6" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"/></svg>
          </button>
        </div>
        <div className="dirA-modal__body">{children}</div>
        {footer && <div className="dirA-modal__foot">{footer}</div>}
      </div>
    </div>
  );
}

function A_AddGroupModal({ open, onClose, onSave }) {
  const [name, setName] = React.useState('');
  const [description, setDescription] = React.useState('');
  const reset = () => { setName(''); setDescription(''); };
  return (
    <A_Modal
      open={open}
      onClose={() => { reset(); onClose(); }}
      title="Create group"
      subtitle="Groups define equipment access and sample visibility scope."
      footer={
        <>
          <button className="btn btn--ghost" onClick={() => { reset(); onClose(); }}>Cancel</button>
          <button className="btn btn--primary" disabled={!name.trim()} onClick={() => { onSave({ name, description }); reset(); onClose(); }}>Create group</button>
        </>
      }
    >
      <div className="field">
        <label>Group name</label>
        <input value={name} onChange={e=>setName(e.target.value)} placeholder="e.g. Quantum Devices" autoFocus/>
        <div className="field-help">Use a short, recognisable lab or PI name.</div>
      </div>
      <div className="field">
        <label>Description (optional)</label>
        <textarea value={description} onChange={e=>setDescription(e.target.value)} placeholder="What does this group work on?" rows="3"/>
      </div>
      <div className="field">
        <label>Equipment access</label>
        <div className="dirA-modal__chips">
          {EQUIPMENT.map(e => (
            <label key={e.id} className="dirA-modal__chip">
              <input type="checkbox" defaultChecked={e.id<=3}/>
              <span>{e.name}</span>
            </label>
          ))}
        </div>
      </div>
    </A_Modal>
  );
}

function A_EditUserModal({ open, onClose, user, onSave }) {
  const [group, setGroup] = React.useState(user?.group || 'Barbaros');
  const [admin, setAdmin] = React.useState(user?.admin || false);
  const [status, setStatus] = React.useState(user?.status || 'approved');
  const [equipment, setEquipment] = React.useState({1:true,2:true,3:true,4:false,5:true,6:false});
  React.useEffect(() => {
    if (user) { setGroup(user.group); setAdmin(user.admin); setStatus(user.status); }
  }, [user]);
  if (!user) return null;
  return (
    <A_Modal
      open={open}
      onClose={onClose}
      title={`Edit ${user.user}`}
      subtitle="Manage role, group, and equipment permissions."
      width={600}
      footer={
        <>
          <button className="btn btn--danger" style={{marginRight:'auto'}}>Suspend user</button>
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={() => { onSave({ ...user, group, admin, status }); onClose(); }}>Save changes</button>
        </>
      }
    >
      <div className="dirA-modal__userhead">
        <div className="avatar" style={{width:44, height:44, fontSize:16}}>{user.user[0].toUpperCase()}</div>
        <div>
          <strong style={{fontSize:15}}>{user.user}</strong>
          <div style={{fontSize:12, color:'var(--text-muted)'}}>{user.user}@lab.example</div>
        </div>
      </div>
      <div className="field-row">
        <div className="field">
          <label>Group</label>
          <select value={group} onChange={e=>setGroup(e.target.value)}>
            {['Barbaros','Ahmet','NewGroup','No group'].map(g=><option key={g}>{g}</option>)}
          </select>
        </div>
        <div className="field">
          <label>Status</label>
          <select value={status} onChange={e=>setStatus(e.target.value)}>
            <option value="approved">Approved</option>
            <option value="pending">Pending approval</option>
            <option value="suspended">Suspended</option>
          </select>
        </div>
      </div>
      <div className="field">
        <label>Role</label>
        <div className="dirA-modal__radios">
          <label className={'dirA-modal__radio' + (!admin ? ' active' : '')}>
            <input type="radio" name="role" checked={!admin} onChange={()=>setAdmin(false)}/>
            <div>
              <strong>Member</strong>
              <div>Create samples, book equipment, edit own work.</div>
            </div>
          </label>
          <label className={'dirA-modal__radio' + (admin ? ' active' : '')}>
            <input type="radio" name="role" checked={admin} onChange={()=>setAdmin(true)}/>
            <div>
              <strong>Admin</strong>
              <div>All member rights, plus user / group / equipment management.</div>
            </div>
          </label>
        </div>
      </div>
      <div className="field">
        <label>Equipment access</label>
        <div className="dirA-modal__chips">
          {EQUIPMENT.map(e => (
            <label key={e.id} className="dirA-modal__chip">
              <input type="checkbox" checked={!!equipment[e.id]} onChange={ev => setEquipment(s => ({...s, [e.id]: ev.target.checked}))}/>
              <span>{e.name}</span>
            </label>
          ))}
        </div>
      </div>
    </A_Modal>
  );
}

function A_BookSlotModal({ open, onClose, onSave, prefill }) {
  const [equipment, setEquipment] = React.useState(prefill?.eq || EQUIPMENT[0].id);
  const [date, setDate] = React.useState('2026-04-19');
  const [start, setStart] = React.useState(prefill?.start ? `${String(Math.floor(prefill.start)).padStart(2,'0')}:${prefill.start%1?'30':'00'}` : '09:00');
  const [end, setEnd] = React.useState(prefill?.end ? `${String(Math.floor(prefill.end)).padStart(2,'0')}:${prefill.end%1?'30':'00'}` : '11:00');
  const [purpose, setPurpose] = React.useState('');
  React.useEffect(() => {
    if (prefill?.eq) setEquipment(prefill.eq);
    if (prefill?.start) setStart(`${String(Math.floor(prefill.start)).padStart(2,'0')}:${prefill.start%1?'30':'00'}`);
    if (prefill?.end) setEnd(`${String(Math.floor(prefill.end)).padStart(2,'0')}:${prefill.end%1?'30':'00'}`);
  }, [prefill]);
  return (
    <A_Modal
      open={open}
      onClose={onClose}
      title="Book equipment slot"
      subtitle="Reserve time on a tool. Conflicts are checked at submit."
      width={560}
      footer={
        <>
          <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={!purpose.trim()} onClick={() => { onSave({ equipment, date, start, end, purpose }); onClose(); }}>Confirm booking</button>
        </>
      }
    >
      <div className="field">
        <label>Equipment</label>
        <select value={equipment} onChange={e=>setEquipment(Number(e.target.value))}>
          {EQUIPMENT.filter(e=>e.access!=='no-access' && e.access!=='admin-view').map(e => <option key={e.id} value={e.id}>{e.name}</option>)}
        </select>
      </div>
      <div className="field-row">
        <div className="field">
          <label>Date</label>
          <input type="date" value={date} onChange={e=>setDate(e.target.value)}/>
        </div>
        <div className="field">
          <label>Start time</label>
          <select value={start} onChange={e=>setStart(e.target.value)}>
            {Array.from({length: 24*4}, (_,i)=>{ const h=Math.floor(i/4), m=(i%4)*15; return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}`; }).map(t=> <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
        <div className="field">
          <label>End time</label>
          <select value={end} onChange={e=>setEnd(e.target.value)}>
            {Array.from({length: 24*4}, (_,i)=>{ const h=Math.floor(i/4), m=(i%4)*15; return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}`; }).map(t=> <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
      </div>
      <div className="field">
        <label>Purpose / sample</label>
        <input value={purpose} onChange={e=>setPurpose(e.target.value)} placeholder="e.g. SMP-042 follow-up Raman"/>
        <div className="field-help">Visible to admins. Helps trace tool usage to projects.</div>
      </div>
      <div className="dirA-modal__notice">
        <strong>Heads up</strong> — Slot will be visible to your group immediately. Cancellations &lt;2h before start time count toward the no-show quota.
      </div>
    </A_Modal>
  );
}

function A_AddEquipmentModal({ open, onClose, onSave }) {
  const [name, setName] = React.useState('');
  const [model, setModel] = React.useState('');
  const [location, setLocation] = React.useState('Cleanroom A');
  const [groups, setGroups] = React.useState({Barbaros:true, Ahmet:false, NewGroup:false});
  const reset = () => { setName(''); setModel(''); setLocation('Cleanroom A'); setGroups({Barbaros:true, Ahmet:false, NewGroup:false}); };
  return (
    <A_Modal
      open={open}
      onClose={() => { reset(); onClose(); }}
      title="Add equipment"
      subtitle="Register a new tool in the lab catalogue."
      width={560}
      footer={
        <>
          <button className="btn btn--ghost" onClick={() => { reset(); onClose(); }}>Cancel</button>
          <button className="btn btn--primary" disabled={!name.trim()} onClick={() => { onSave({ name, model, location, groups }); reset(); onClose(); }}>Add equipment</button>
        </>
      }
    >
      <div className="field">
        <label>Equipment name</label>
        <input value={name} onChange={e=>setName(e.target.value)} placeholder="e.g. Raman Spectrometer 2" autoFocus/>
      </div>
      <div className="field-row">
        <div className="field">
          <label>Model / part #</label>
          <input value={model} onChange={e=>setModel(e.target.value)} placeholder="Renishaw inVia"/>
        </div>
        <div className="field">
          <label>Location</label>
          <select value={location} onChange={e=>setLocation(e.target.value)}>
            <option>Cleanroom A</option><option>Cleanroom B</option><option>Lab 1.04</option><option>Lab 2.12</option>
          </select>
        </div>
      </div>
      <div className="field">
        <label>Group access</label>
        <div className="dirA-modal__chips">
          {Object.keys(groups).map(g => (
            <label key={g} className="dirA-modal__chip">
              <input type="checkbox" checked={groups[g]} onChange={ev => setGroups(s => ({...s, [g]: ev.target.checked}))}/>
              <span>{g}</span>
            </label>
          ))}
        </div>
        <div className="field-help">Only members of the selected groups can book this equipment.</div>
      </div>
    </A_Modal>
  );
}

Object.assign(window, { A_Modal, A_AddGroupModal, A_EditUserModal, A_BookSlotModal, A_AddEquipmentModal });


// ===========================================================
// Sample detail action modals
// ===========================================================

function A_AddKeywordModal({ open, onClose, onAdd, existing = [] }) {
  const [val, setVal] = React.useState('');
  const [picked, setPicked] = React.useState([]);
  React.useEffect(() => { if (open) { setVal(''); setPicked([]); } }, [open]);
  const SUGGEST = ['MBE','CVD','heterostructure','quantum','2D','photovoltaic','superconductor','low-T','optical','electrical','spectroscopy','characterization','annealed'];
  const togglePick = (k) => setPicked(p => p.includes(k) ? p.filter(x=>x!==k) : [...p,k]);
  const submit = () => {
    const all = [...picked];
    const trimmed = val.trim();
    if (trimmed && !all.includes(trimmed)) all.push(trimmed);
    if (all.length) onAdd(all);
    onClose();
  };
  return (
    <A_Modal open={open} onClose={onClose} title="Add keywords" subtitle="Tag this sample to make it easier to find. Comma-separate multiple keywords or pick from suggestions." width={500}
      footer={<><button className="btn btn--ghost" onClick={onClose}>Cancel</button><button className="btn btn--primary" onClick={submit} disabled={!val.trim() && !picked.length}>Add {picked.length + (val.trim()?1:0)} keyword{(picked.length + (val.trim()?1:0))===1?'':'s'}</button></>}>
      <div className="field">
        <label>New keyword</label>
        <input autoFocus value={val} onChange={e=>setVal(e.target.value)} placeholder="e.g. graphene, MBE, transport" onKeyDown={e=>{if(e.key==='Enter')submit();}}/>
      </div>
      <div>
        <label style={{fontSize:12, fontWeight:600, color:'var(--text-muted)', display:'block', marginBottom:8, textTransform:'uppercase', letterSpacing:'0.04em'}}>Suggestions</label>
        <div className="dirA-modal__chips">
          {SUGGEST.filter(s => !existing.includes(s)).map(s => (
            <label key={s} className="dirA-modal__chip">
              <input type="checkbox" checked={picked.includes(s)} onChange={()=>togglePick(s)}/>
              {s}
            </label>
          ))}
        </div>
      </div>
    </A_Modal>
  );
}

function A_EditSampleModal({ open, onClose, sample, onSave }) {
  const s = sample || {};
  const [name, setName] = React.useState(s.name || '');
  const [status, setStatus] = React.useState(s.status || 'active');
  const [owner, setOwner] = React.useState(s.owner || '');
  const [palette, setPalette] = React.useState(s.palette || 'blue');
  React.useEffect(() => { if (open && sample) { setName(sample.name); setStatus(sample.status); setOwner(sample.owner); setPalette(sample.palette); } }, [open, sample]);
  const submit = () => { onSave({ ...sample, name, status, owner, palette }); onClose(); };
  return (
    <A_Modal open={open} onClose={onClose} title="Edit sample" subtitle={s.id ? `Update sample ${s.id}` : 'Update sample details'} width={540}
      footer={<><button className="btn btn--ghost" onClick={onClose}>Cancel</button><button className="btn btn--primary" onClick={submit}>Save changes</button></>}>
      <div className="field"><label>Name</label><input autoFocus value={name} onChange={e=>setName(e.target.value)}/></div>
      <div className="field-row">
        <div className="field"><label>Owner</label>
          <select value={owner} onChange={e=>setOwner(e.target.value)}>{['msmith','j.chen','a.patel','k.nakamura','r.ivanov','l.okafor'].map(u=><option key={u}>{u}</option>)}</select>
        </div>
        <div className="field"><label>Status</label>
          <select value={status} onChange={e=>setStatus(e.target.value)}>
            <option value="active">Active</option><option value="progress">In Progress</option>
            <option value="archived">Archived</option><option value="pending">Pending</option>
          </select>
        </div>
      </div>
      <div className="field"><label>Thumbnail palette</label>
        <div className="dirA-modal__chips">
          {['blue','teal','violet','emerald','amber','rose'].map(p => (
            <label key={p} className="dirA-modal__chip" style={{borderColor: palette===p ? 'var(--color-primary)' : undefined, background: palette===p ? 'rgba(37,99,235,0.08)' : undefined, color: palette===p ? 'var(--color-primary)' : undefined, fontWeight: palette===p?600:400}}>
              <input type="radio" name="palette" checked={palette===p} onChange={()=>setPalette(p)}/>{p}
            </label>
          ))}
        </div>
      </div>
    </A_Modal>
  );
}

function A_ShareSampleModal({ open, onClose, sample }) {
  const s = sample || {};
  const link = typeof window !== 'undefined' ? `${window.location.origin}/samples/${s.id || 'SMP-000'}` : '';
  const [copied, setCopied] = React.useState(false);
  const [scope, setScope] = React.useState('group');
  const [users, setUsers] = React.useState(['j.chen']);
  React.useEffect(() => { if (open) { setCopied(false); setScope('group'); setUsers(['j.chen']); } }, [open]);
  const toggleUser = (u) => setUsers(arr => arr.includes(u) ? arr.filter(x=>x!==u) : [...arr,u]);
  const copy = () => {
    if (navigator.clipboard) navigator.clipboard.writeText(link).catch(()=>{});
    setCopied(true); setTimeout(()=>setCopied(false), 1500);
  };
  return (
    <A_Modal open={open} onClose={onClose} title="Share sample" subtitle={`Give others access to ${s.id || 'this sample'}`} width={540}
      footer={<><button className="btn btn--ghost" onClick={onClose}>Close</button><button className="btn btn--primary" onClick={onClose}>Done</button></>}>
      <div className="field"><label>Direct link</label>
        <div style={{display:'flex', gap: 8}}>
          <input readOnly value={link} style={{flex:1, fontFamily:'var(--font-family-mono)', fontSize:12.5}}/>
          <button className="btn btn--secondary" onClick={copy}>{copied ? 'Copied ✓' : 'Copy'}</button>
        </div>
      </div>
      <div>
        <label style={{fontSize:12, fontWeight:600, color:'var(--text-muted)', display:'block', marginBottom:10, textTransform:'uppercase', letterSpacing:'0.04em'}}>Access</label>
        <div className="dirA-modal__radios">
          {[
            {v:'private', t:'Private', d:'Only you and admins can see this sample.'},
            {v:'group',   t:'My group (Barbaros)', d:'All members of your lab group can view.'},
            {v:'lab',     t:'Whole lab', d:'Anyone in the institution with access to SampleDB.'},
            {v:'link',    t:'Anyone with the link', d:'Read-only public access — useful for collaborators outside the lab.'},
          ].map(o => (
            <label key={o.v} className={'dirA-modal__radio ' + (scope===o.v?'active':'')}>
              <input type="radio" name="scope" checked={scope===o.v} onChange={()=>setScope(o.v)}/>
              <div><strong>{o.t}</strong><div>{o.d}</div></div>
            </label>
          ))}
        </div>
      </div>
      <div className="field">
        <label>Invite specific users</label>
        <div className="dirA-modal__chips">
          {['j.chen','a.patel','k.nakamura','r.ivanov','l.okafor'].map(u => (
            <label key={u} className="dirA-modal__chip">
              <input type="checkbox" checked={users.includes(u)} onChange={()=>toggleUser(u)}/>{u}
            </label>
          ))}
        </div>
      </div>
    </A_Modal>
  );
}

function A_DeleteConfirmModal({ open, onClose, onConfirm, title='Delete?', body='This cannot be undone.', confirmLabel='Delete', danger=true }) {
  const [val, setVal] = React.useState('');
  const requiresType = typeof body === 'object' && body.requireType;
  const expected = requiresType ? body.requireType : null;
  React.useEffect(() => { if (open) setVal(''); }, [open]);
  return (
    <A_Modal open={open} onClose={onClose} title={title} width={460}
      footer={<><button className="btn btn--ghost" onClick={onClose}>Cancel</button><button className={'btn ' + (danger?'btn--danger':'btn--primary')} onClick={()=>{ onConfirm(); onClose(); }} disabled={requiresType && val !== expected} style={danger?{background:'var(--color-destructive)', color:'#fff', borderColor:'var(--color-destructive)'}:undefined}>{confirmLabel}</button></>}>
      <div style={{fontSize: 14, lineHeight: 1.55, color:'var(--text-strong)'}}>
        {requiresType ? body.message : body}
      </div>
      {requiresType && (
        <div className="field">
          <label>Type <code style={{background:'var(--surface-muted)', padding:'1px 6px', borderRadius:4, fontFamily:'var(--font-family-mono)'}}>{expected}</code> to confirm</label>
          <input autoFocus value={val} onChange={e=>setVal(e.target.value)}/>
        </div>
      )}
    </A_Modal>
  );
}

function A_StepModal({ open, onClose, onSave, step, index, mode }) {
  const [title, setTitle] = React.useState('');
  const [date, setDate] = React.useState('');
  const [by, setBy] = React.useState('msmith');
  const [tool, setTool] = React.useState('');
  const [notes, setNotes] = React.useState('');
  React.useEffect(() => {
    if (!open) return;
    if (step) { setTitle(step.title||''); setDate(step.date||''); setBy(step.by||'msmith'); setTool(step.tool||''); setNotes(step.notes||''); }
    else { setTitle(''); setDate(new Date().toISOString().slice(0,10)); setBy('msmith'); setTool(''); setNotes(''); }
  }, [open, step]);
  const submit = () => { onSave({ title, date, by, tool, notes }, index); onClose(); };
  const isEdit = mode === 'edit';
  return (
    <A_Modal open={open} onClose={onClose} title={isEdit ? `Edit step ${String((index||0)+1).padStart(2,'0')}` : 'Add fabrication step'} subtitle={isEdit ? 'Update the recipe entry for this fabrication step.' : 'Record a new step in the fabrication recipe.'} width={580}
      footer={<><button className="btn btn--ghost" onClick={onClose}>Cancel</button><button className="btn btn--primary" onClick={submit} disabled={!title.trim()}>{isEdit ? 'Save step' : 'Add step'}</button></>}>
      <div className="field"><label>Step title</label><input autoFocus value={title} onChange={e=>setTitle(e.target.value)} placeholder="e.g. CA2DM Optical Lithography"/></div>
      <div className="field-row">
        <div className="field"><label>Date</label><input type="date" value={date} onChange={e=>setDate(e.target.value)}/></div>
        <div className="field"><label>Performed by</label>
          <select value={by} onChange={e=>setBy(e.target.value)}>{['msmith','j.chen','a.patel','k.nakamura','r.ivanov','l.okafor'].map(u=><option key={u}>{u}</option>)}</select>
        </div>
        <div className="field"><label>Tool / station</label><input value={tool} onChange={e=>setTool(e.target.value)} placeholder="e.g. Bench 3, MBE-A"/></div>
      </div>
      <div className="field"><label>Notes <span style={{fontWeight:400, color:'var(--text-muted)'}}>(optional)</span></label>
        <textarea value={notes} onChange={e=>setNotes(e.target.value)} placeholder="Recipe parameters, observations, deviations..." style={{minHeight: 90, resize:'vertical'}}/>
      </div>
    </A_Modal>
  );
}

Object.assign(window, { A_AddKeywordModal, A_EditSampleModal, A_ShareSampleModal, A_DeleteConfirmModal, A_StepModal });
