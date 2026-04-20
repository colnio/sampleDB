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
          {EQUIPMENT.map(e => <option key={e.id} value={e.id}>{e.name}</option>)}
        </select>
      </div>
      <div className="field-row">
        <div className="field">
          <label>Date</label>
          <input type="date" value={date} onChange={e=>setDate(e.target.value)}/>
        </div>
        <div className="field">
          <label>Start time</label>
          <input type="time" value={start} onChange={e=>setStart(e.target.value)}/>
        </div>
        <div className="field">
          <label>End time</label>
          <input type="time" value={end} onChange={e=>setEnd(e.target.value)}/>
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
