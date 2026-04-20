// Direction A — Refined: classic card + sidebar shell
const dirAStatusChip = (s) => {
  const map = { active: 'status-chip--active', progress: 'status-chip--progress', archived: 'status-chip--archived', pending: 'status-chip--pending' };
  const label = s === 'progress' ? 'In Progress' : s[0].toUpperCase() + s.slice(1);
  return <span className={`status-chip ${map[s]||''}`}><span className="dot"/>{label}</span>;
};

function A_Sidebar({ screen, setScreen }) {
  const items = [
    { id: 'dashboard', label: 'Dashboard', icon: Ic.Dashboard },
    { id: 'samples', label: 'Samples', icon: Ic.Samples, count: 247 },
    { id: 'wiki', label: 'Wiki', icon: Ic.Wiki, count: 42 },
    { id: 'booking', label: 'Equipment', icon: Ic.Calendar },
    { id: 'tokens', label: 'API Tokens', icon: Ic.Key },
    { id: 'admin', label: 'Admin', icon: Ic.Shield },
  ];
  return (
    <aside className="sidebar">
      <div className="sidebar__brand"><Ic.Flask style={{color:'var(--color-primary)'}}/><span>Sample Tracker</span></div>
      <button className="btn btn--primary" style={{width:'100%', marginBottom: 10}} onClick={() => setScreen('new')}><Ic.Plus/> New Sample</button>
      <div className="nav-group">
        <div className="nav-group__title">Workspace</div>
        {items.map(it => (
          <button key={it.id} className={'nav-link' + (screen===it.id?' active':'')} onClick={()=>setScreen(it.id)}>
            <it.icon/><span>{it.label}</span>{it.count!=null && <span className="count">{it.count}</span>}
          </button>
        ))}
      </div>
      <div className="sidebar__footer">
        <div className="avatar">MS</div>
        <div style={{flex:1, fontSize:13}}>
          <div style={{fontWeight:600}}>msmith</div>
          <div style={{fontSize:11, color:'var(--text-muted)'}}>Admin · Barbaros</div>
        </div>
      </div>
    </aside>
  );
}

function A_Dashboard({ setScreen, setActiveSample }) {
  return (
    <div>
      <div className="topbar">
        <div className="topbar__left">
          <h1>Welcome back, msmith</h1>
          <div className="crumbs">Friday · April 19, 2026</div>
        </div>
        <div className="topbar__actions">
          <button className="btn btn--ghost"><Ic.Download/> Export</button>
          <button className="btn btn--primary" onClick={()=>setScreen('new')}><Ic.Plus/> New Sample</button>
        </div>
      </div>
      <div className="stats">
        {[
          {l:'Total Samples',v:'247',d:'+12 this week'},
          {l:'Active Bookings',v:'8',d:'2 upcoming today'},
          {l:'Wiki Articles',v:'42',d:'+3 this week'},
          {l:'Pending Approvals',v:'3',d:'Requires action', down:true},
        ].map((s,i)=>(
          <div key={i} className="stat-card">
            <div className="stat-card__label">{s.l}</div>
            <div className="stat-card__value">{s.v}</div>
            <div className={'stat-card__delta'+(s.down?' down':'')}>{s.d}</div>
          </div>
        ))}
      </div>
      <div style={{display:'grid', gridTemplateColumns: '2fr 1fr', gap: 18}}>
        <div className="card" style={{padding: '20px 24px'}}>
          <div className="u-between" style={{marginBottom: 16}}>
            <h3 style={{margin:0,fontSize:15}}>Recent samples</h3>
            <a style={{fontSize:12.5, cursor:'pointer'}} onClick={()=>setScreen('samples')}>View all →</a>
          </div>
          <div style={{display:'flex', flexDirection:'column', gap: 8}}>
            {SAMPLES.slice(0,5).map(s=>(
              <div key={s.id} className="u-between" style={{padding:'10px 12px', borderRadius:8, cursor:'pointer'}}
                   onMouseOver={(e)=>e.currentTarget.style.background='var(--surface-subtle)'}
                   onMouseOut={(e)=>e.currentTarget.style.background='transparent'}
                   onClick={()=>{ setActiveSample(s); setScreen('detail'); }}>
                <div className="u-center u-gap-md">
                  <div style={{width:36,height:36, borderRadius:8, overflow:'hidden'}}><SampleThumb seed={parseInt(s.id.slice(-3),36)} palette={s.palette}/></div>
                  <div>
                    <div style={{fontWeight:600, fontSize:13.5}}>{s.name}</div>
                    <div style={{fontSize:12, color:'var(--text-muted)', fontFamily:'var(--font-family-mono)'}}>{s.id} · {s.owner}</div>
                  </div>
                </div>
                <div className="u-center u-gap-md">
                  {dirAStatusChip(s.status)}
                  <div style={{fontSize:12, color:'var(--text-muted)', minWidth: 76, textAlign:'right'}}>{s.createdAt}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="card" style={{padding: '20px 24px'}}>
          <h3 style={{margin:'0 0 12px',fontSize:15}}>Activity</h3>
          {ACTIVITY.slice(0,6).map((a,i)=>(
            <div key={i} className="activity-item">
              <div className="dot-col"/>
              <div style={{flex:1}}>
                <strong>{a.who}</strong> {a.verb} <a>{a.what}</a>
              </div>
              <div className="time">{a.when}</div>
            </div>
          ))}
        </div>
      </div>
      <div className="card" style={{padding:'20px 24px', marginTop: 18}}>
        <div className="u-between" style={{marginBottom:14}}>
          <h3 style={{margin:0, fontSize: 15}}>Today's bookings</h3>
          <a style={{fontSize:12.5, cursor:'pointer'}} onClick={()=>setScreen('booking')}>Open schedule →</a>
        </div>
        <A_MiniBooking />
      </div>
    </div>
  );
}

function A_MiniBooking() {
  const hours = Array.from({length: 12}, (_,i) => i+8);
  return (
    <div className="timeline-wrap">
      <div className="timeline-wrap__scroll">
        <div className="tline">
          <div className="tline-head">
            <div className="tline-head__left">Equipment</div>
            <div className="tline-hours">{hours.map(h => <div key={h} className="tline-hour">{h}:00</div>)}</div>
          </div>
          {EQUIPMENT.slice(0,4).map(eq => (
            <div className="tline-row" key={eq.id}>
              <div className="tline-row__left"><span className="tline-row__dot"/>{eq.name}</div>
              <div className="tline-lane">
                <div className="slots">{hours.map(h => <div key={h} className="slot"/>)}</div>
                {BOOKINGS.filter(b=>b.eq===eq.id).map((b,i)=>{
                  const left = (b.start - 8) * 80;
                  const w = (b.end - b.start) * 80;
                  return <div key={i} className={'tline-block'+(b.own?' tline-block--own':'')} style={{left, width: w-4}}>
                    <div>{b.user}</div>
                    <small>{b.purpose}</small>
                  </div>;
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function A_Samples({ setScreen, setActiveSample }) {
  const [view, setView] = React.useState('grid');
  const [status, setStatus] = React.useState('all');
  const filtered = SAMPLES.filter(s => status==='all' || s.status === status);
  return (
    <div>
      <div className="topbar">
        <div className="topbar__left"><h1>Samples</h1><div className="crumbs">{filtered.length} of {SAMPLES.length} samples</div></div>
        <div className="topbar__actions">
          <button className="btn btn--ghost"><Ic.Download/> Export CSV</button>
          <button className="btn btn--primary" onClick={()=>setScreen('new')}><Ic.Plus/> New Sample</button>
        </div>
      </div>
      <div className="toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search samples, keywords, owners…"/></div>
        <div className="filter-chip"><Ic.Calendar/> Apr 01 – Apr 19</div>
        <div className="filter-chip"><Ic.Filter/> All owners</div>
        {['all','active','progress','archived'].map(s => (
          <button key={s} className={'filter-chip'+(status===s?' active':'')} onClick={()=>setStatus(s)} style={{border:'none'}}>
            {s==='all'?'All status':s==='progress'?'In Progress':s[0].toUpperCase()+s.slice(1)}
          </button>
        ))}
        <div style={{flex:1}}/>
        <div className="seg">
          <button className={view==='grid'?'active':''} onClick={()=>setView('grid')}><Ic.Grid/>Grid</button>
          <button className={view==='table'?'active':''} onClick={()=>setView('table')}><Ic.List/>Table</button>
        </div>
      </div>
      {view==='grid' ? (
        <div className="samples-grid">
          {filtered.map(s => (
            <article key={s.id} className="card sample-card" onClick={()=>{ setActiveSample(s); setScreen('detail'); }}>
              <div className="sample-card__thumb">
                <span className="status-chip status">{dirAStatusChip(s.status).props.children[1]}</span>
                <SampleThumb seed={parseInt(s.id.slice(-3),36)} palette={s.palette}/>
              </div>
              <div className="sample-card__body">
                <div className="sample-card__id">{s.id}</div>
                <h3 className="sample-card__title">{s.name}</h3>
                <p className="sample-card__desc">{s.description}</p>
                <div style={{display:'flex', flexWrap:'wrap', gap: 4, marginTop: 4}}>
                  {s.keywords.map(k => <span key={k} style={{fontSize:11, padding:'2px 8px', borderRadius:999, background:'var(--surface-muted)', color:'var(--text-muted)'}}>{k}</span>)}
                </div>
                <div className="sample-card__meta"><span>{s.owner}</span><span>· {s.createdAt}</span><span>· {s.steps} steps</span></div>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <table className="data-table">
          <thead><tr>
            <th style={{width:100}}>ID</th><th>Name</th><th>Status</th><th>Owner</th><th>Steps</th><th>Files</th><th>Created</th><th></th>
          </tr></thead>
          <tbody>
            {filtered.map(s => (
              <tr key={s.id} style={{cursor:'pointer'}} onClick={()=>{ setActiveSample(s); setScreen('detail'); }}>
                <td><span className="sample-id">{s.id}</span></td>
                <td><strong>{s.name}</strong><div style={{fontSize:12, color:'var(--text-muted)', marginTop:2}}>{s.keywords.join(', ')}</div></td>
                <td>{dirAStatusChip(s.status)}</td>
                <td>{s.owner}</td>
                <td>{s.steps}</td>
                <td>{s.attachments}</td>
                <td style={{fontFamily:'var(--font-family-mono)', fontSize:12, color:'var(--text-muted)'}}>{s.createdAt}</td>
                <td><button className="btn btn--ghost btn--sm"><Ic.Edit/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function A_SampleDetail({ sample, setScreen }) {
  const s = sample || SAMPLES[0];
  const [editing, setEditing] = React.useState(null);
  const [name, setName] = React.useState(s.name);
  const [desc, setDesc] = React.useState(s.description);
  return (
    <div>
      <a style={{fontSize:12.5, color:'var(--text-muted)', cursor:'pointer', display:'inline-flex', alignItems:'center', gap: 6, marginBottom: 10}} onClick={()=>setScreen('samples')}><Ic.ArrowLeft style={{width:14,height:14}}/> Back to samples</a>
      <div className="topbar" style={{marginBottom: 20}}>
        <div className="topbar__left" style={{flex:1}}>
          <div className="u-center u-gap-md" style={{marginBottom: 4}}>
            <span style={{fontFamily:'var(--font-family-mono)', fontSize:12, color:'var(--text-muted)'}}>{s.id}</span>
            {dirAStatusChip(s.status)}
          </div>
          {editing==='name' ? (
            <input autoFocus className="u-mono" style={{fontSize: 22, fontWeight: 700, width: '100%', padding: '4px 8px', border:'1px solid var(--color-primary)', borderRadius: 8, background:'var(--surface-default)', color:'var(--text-strong)'}} value={name} onChange={e=>setName(e.target.value)} onBlur={()=>setEditing(null)} onKeyDown={e=>{if(e.key==='Enter')setEditing(null)}}/>
          ) : (
            <h1 onClick={()=>setEditing('name')} style={{cursor:'text', color:'var(--text-strong)'}} title="Click to edit">{name}</h1>
          )}
          <div className="crumbs">Created {s.createdAt} · Owner <strong>{s.owner}</strong> · {s.attachments} files · {s.steps} fabrication steps</div>
        </div>
        <div className="topbar__actions">
          <button className="btn btn--ghost"><Ic.Download/> Export</button>
          <button className="btn btn--secondary"><Ic.Edit/> Edit</button>
          <button className="btn btn--primary"><Ic.Sparkle/> Share</button>
        </div>
      </div>
      <div style={{display:'grid', gridTemplateColumns:'2fr 1fr', gap: 18}}>
        <div style={{display:'flex', flexDirection:'column', gap:18}}>
          <div className="card" style={{padding:'18px 22px'}}>
            <div className="u-eyebrow" style={{marginBottom: 10}}>Description</div>
            {editing==='desc' ? (
              <textarea autoFocus value={desc} onChange={e=>setDesc(e.target.value)} onBlur={()=>setEditing(null)} style={{width:'100%', minHeight: 120, padding: 10, borderRadius: 8, border:'1px solid var(--color-primary)', background:'var(--surface-default)', color:'var(--text-strong)', fontFamily:'inherit', fontSize: 14, resize:'vertical'}}/>
            ) : (
              <p style={{margin:0, lineHeight: 1.7, cursor:'text'}} onClick={()=>setEditing('desc')} title="Click to edit">{desc}</p>
            )}
            <div style={{display:'flex', gap:6, marginTop: 14, flexWrap:'wrap'}}>
              {s.keywords.map(k=> <span key={k} style={{fontSize:12, padding:'3px 10px', borderRadius:999, background:'var(--color-secondary-bg)', color:'var(--color-secondary-text)', fontWeight:500}}>{k}</span>)}
              <span style={{fontSize:12, padding:'3px 10px', borderRadius:999, border:'1px dashed var(--border-subtle)', color:'var(--text-muted)', cursor:'pointer'}}>+ Add</span>
            </div>
          </div>
          <div className="card" style={{padding:'18px 22px'}}>
            <div className="u-between" style={{marginBottom: 14}}>
              <div className="u-eyebrow">Fabrication Steps</div>
              <button className="btn btn--ghost btn--sm"><Ic.Plus/> Add Step</button>
            </div>
            <div className="fab-timeline">
              {STEPS.map((st, i) => (
                <div key={i} className="fab-step">
                  <div style={{display:'flex', alignItems:'center', gap: 8}}>
                    <Ic.Drag style={{width: 12}} className="fab-step__grab"/>
                    <div className="fab-step__num">{String(i+1).padStart(2,'0')}</div>
                  </div>
                  <div>
                    <div className="fab-step__title">{st.title}</div>
                    <div className="fab-step__meta">
                      <span>📅 {st.date}</span><span>👤 {st.by}</span><span>🔧 {st.tool}</span>
                      {st.notes && <span style={{fontStyle:'italic'}}>{st.notes}</span>}
                    </div>
                    <div className="fab-step__actions">
                      <button className="btn btn--ghost btn--sm"><Ic.Edit/></button>
                      <button className="btn btn--danger btn--sm"><Ic.Trash/></button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{padding:'18px 22px'}}>
            <div className="u-between" style={{marginBottom: 14}}>
              <div className="u-eyebrow">Attachments ({s.attachments})</div>
              <button className="btn btn--ghost btn--sm"><Ic.Upload/> Upload</button>
            </div>
            <div style={{display:'grid', gridTemplateColumns:'repeat(auto-fill,minmax(140px,1fr))', gap: 10}}>
              {Array.from({length: 5}).map((_,i)=>(
                <div key={i} style={{height:100, borderRadius:8, overflow:'hidden', border:'1px solid var(--border-subtle)'}}>
                  <SampleThumb seed={i+s.id.length} palette={s.palette}/>
                </div>
              ))}
              <div style={{height:100, borderRadius:8, border:'1px dashed var(--border-subtle)', display:'flex', alignItems:'center', justifyContent:'center', color:'var(--text-muted)', fontSize:12, flexDirection:'column', gap: 4}}>
                <Ic.Upload style={{width:20,height:20}}/> Drop files
              </div>
            </div>
          </div>
        </div>
        <div style={{display:'flex', flexDirection:'column', gap:18}}>
          <div className="card" style={{padding:'18px 22px'}}>
            <div className="u-eyebrow" style={{marginBottom: 12}}>Details</div>
            {[
              ['Owner', s.owner], ['Status', dirAStatusChip(s.status)],
              ['Created', s.createdAt], ['Last edit', '2h ago by msmith'],
              ['Steps', s.steps], ['Files', s.attachments],
            ].map(([k,v],i)=>(
              <div key={i} style={{display:'flex', justifyContent:'space-between', padding:'7px 0', borderBottom:'1px solid var(--border-subtle)', fontSize:13}}>
                <span style={{color:'var(--text-muted)'}}>{k}</span><span style={{fontWeight:500}}>{v}</span>
              </div>
            ))}
          </div>
          <div className="card" style={{padding:'18px 22px'}}>
            <div className="u-eyebrow" style={{marginBottom: 12}}>Activity</div>
            {ACTIVITY.slice(0,5).map((a,i)=>(
              <div key={i} className="activity-item" style={{padding:'6px 0'}}>
                <div className="dot-col"/>
                <div style={{flex:1, fontSize:12.5}}><strong>{a.who}</strong> {a.verb}</div>
                <div className="time">{a.when}</div>
              </div>
            ))}
          </div>
          <div className="card" style={{padding:'18px 22px', borderTopColor: 'var(--color-destructive)'}}>
            <div className="u-eyebrow" style={{marginBottom: 8, color:'var(--color-destructive)'}}>Danger Zone</div>
            <p style={{fontSize:12.5, color:'var(--text-muted)', margin:'0 0 10px'}}>Permanently delete this sample and all fabrication steps + attachments.</p>
            <button className="btn btn--danger"><Ic.Trash/> Delete sample</button>
          </div>
        </div>
      </div>
    </div>
  );
}

function A_New({ setScreen }) {
  return (
    <div>
      <a style={{fontSize:12.5, color:'var(--text-muted)', cursor:'pointer', display:'inline-flex', alignItems:'center', gap: 6, marginBottom: 10}} onClick={()=>setScreen('samples')}><Ic.ArrowLeft style={{width:14}}/> Back to samples</a>
      <div className="topbar"><div className="topbar__left"><h1>New Sample</h1><div className="crumbs">Step 1 of 2 · Details → Fabrication steps</div></div></div>
      <div className="card form-card" style={{maxWidth: 860}}>
        <h3 style={{margin:'0 0 4px', fontSize: 15, color: 'var(--color-primary)'}}>Sample Details</h3>
        <p style={{color:'var(--text-muted)', fontSize:13, margin:'0 0 20px'}}>Fill in the basics. You'll add fabrication steps after saving.</p>
        <div className="field-row">
          <div className="field"><label>Sample ID</label><input placeholder="SMP-043"/><div className="field-help">Auto-generated, editable.</div></div>
          <div className="field"><label>Sample Name</label><input placeholder="e.g. GaAs Wafer Batch 043"/></div>
        </div>
        <div className="field"><label>Description</label><textarea placeholder="Growth conditions, substrate, key parameters…"/></div>
        <div className="field-row">
          <div className="field"><label>Owner</label><select><option>msmith</option><option>j.chen</option><option>-- Unassigned --</option></select></div>
          <div className="field"><label>Status</label><select><option>Active</option><option>In Progress</option><option>Archived</option></select></div>
        </div>
        <div className="field"><label>Keywords (comma separated)</label><input placeholder="GaAs, MBE, heterostructure"/></div>
        <div style={{display:'flex', justifyContent:'flex-end', gap: 10, marginTop: 8}}>
          <button className="btn btn--ghost" onClick={()=>setScreen('samples')}>Cancel</button>
          <button className="btn btn--primary">Save & add steps →</button>
        </div>
      </div>
    </div>
  );
}

function A_Wiki({ setScreen }) {
  return (
    <div>
      <div className="topbar">
        <div className="topbar__left"><h1>Wiki</h1><div className="crumbs">Protocols, equipment guides, and lab conventions</div></div>
        <div className="topbar__actions">
          <button className="btn btn--primary"><Ic.Plus/> New Article</button>
        </div>
      </div>
      <div className="toolbar">
        <div className="search"><Ic.Search/><input placeholder="Search articles…"/></div>
        {['All','Safety','Equipment','Protocols','Admin'].map(t => <span key={t} className={'filter-chip'+(t==='All'?' active':'')}>{t}</span>)}
      </div>
      <div style={{display:'grid', gridTemplateColumns:'repeat(auto-fill,minmax(300px,1fr))', gap: 16}}>
        {ARTICLES.map(a => (
          <div key={a.slug} className="card" style={{padding:'18px 22px', cursor:'pointer'}} onClick={()=>setScreen('wikiArticle')}>
            <div style={{display:'flex', alignItems:'center', gap:8, marginBottom: 10}}>
              <span className="status-chip status-chip--progress">{a.tag}</span>
              <span style={{fontSize:12, color:'var(--text-muted)'}}>{a.createdAt}</span>
            </div>
            <h3 style={{margin:'0 0 6px', fontSize: 16, color:'var(--color-primary)'}}>{a.title}</h3>
            <p style={{margin:0, fontSize:13, color:'var(--text-muted)', lineHeight: 1.55}}>{a.summary}</p>
            <div style={{marginTop:12, fontSize:12, color:'var(--text-muted)', display:'flex', alignItems:'center', gap:6}}>
              <div className="avatar" style={{width:22, height:22, fontSize: 11}}>{a.author[0].toUpperCase()}</div>
              {a.author}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function A_WikiArticle({ setScreen }) {
  return (
    <div>
      <a style={{fontSize:12.5, color:'var(--text-muted)', cursor:'pointer', display:'inline-flex', alignItems:'center', gap: 6, marginBottom: 10}} onClick={()=>setScreen('wiki')}><Ic.ArrowLeft style={{width:14}}/> Back to Wiki</a>
      <div className="topbar">
        <div className="topbar__left">
          <div className="u-center u-gap-sm" style={{marginBottom: 4}}>
            <span className="status-chip status-chip--progress">Safety</span>
            <span style={{fontSize:12, color:'var(--text-muted)'}}>Nov 10, 2024 · msmith</span>
          </div>
          <h1>Cleanroom Safety Checklist</h1>
        </div>
        <div className="topbar__actions">
          <button className="btn btn--secondary"><Ic.Edit/> Edit</button>
          <button className="btn btn--ghost"><Ic.Download/> Export</button>
        </div>
      </div>
      <div className="wiki-grid" style={{display:'grid', gridTemplateColumns: '3fr 1fr', gap: 18}}>
        <div className="card" style={{padding:'24px 32px', fontSize: 15, lineHeight: 1.75}}>
          <h2 style={{fontSize: 18, marginTop: 0}}>Entering the cleanroom</h2>
          <ol>
            <li>Sign in at the access terminal using your lab badge.</li>
            <li>Remove outerwear and store in the assigned locker.</li>
            <li>Gown up in this order: booties → bouffant → mask → coverall → gloves.</li>
          </ol>
          <h2 style={{fontSize: 18}}>Chemical handling</h2>
          <p>Any use of HF, KOH, or solvents above 10 mL must be logged in the wet bench registry. Double-glove and confirm the emergency shower is unobstructed.</p>
          <pre style={{background:'var(--surface-muted)', padding: 14, borderRadius: 8, fontSize: 13, overflow:'auto'}}>{`# Wet-bench checklist
[ ] PPE verified
[ ] Ventilation on
[ ] Spill kit accessible
[ ] Logged in registry`}</pre>
          <h2 style={{fontSize: 18}}>Emergency contacts</h2>
          <p>EHS on-call: <strong>x2500</strong> · Lab manager: <strong>msmith</strong></p>

          <div className="wiki-attachments">
            <div className="u-between" style={{marginBottom: 12}}>
              <h3 style={{margin:0, fontSize: 15}}>Attachments <span style={{color:'var(--text-muted)', fontWeight: 500}}>· 4</span></h3>
              <button className="btn btn--secondary btn--sm"><Ic.Plus/> Upload</button>
            </div>
            <div className="wiki-attach-list">
              {[
                {name:'cleanroom-floorplan.pdf', size:'1.4 MB', kind:'PDF', by:'msmith', when:'Apr 14'},
                {name:'gowning-procedure.mp4',   size:'24.2 MB', kind:'VIDEO', by:'msmith', when:'Apr 14'},
                {name:'wet-bench-registry.xlsx', size:'82 KB', kind:'XLSX', by:'j.chen', when:'Apr 12'},
                {name:'sds-hf-acid.pdf',         size:'310 KB', kind:'PDF', by:'msmith', when:'Mar 30'},
              ].map(f => (
                <div key={f.name} className="wiki-attach">
                  <div className={'wiki-attach__icon wiki-attach__icon--' + f.kind.toLowerCase()}>{f.kind}</div>
                  <div className="wiki-attach__meta">
                    <strong>{f.name}</strong>
                    <div>{f.size} · uploaded by {f.by} · {f.when}</div>
                  </div>
                  <button className="btn btn--ghost btn--sm" title="Download"><Ic.Download/></button>
                  <button className="btn btn--ghost btn--sm" title="Remove"><Ic.Trash/></button>
                </div>
              ))}
              <div className="wiki-attach-drop">
                <Ic.Plus style={{width:18, height:18, opacity:.6}}/>
                <span>Drop files here or click <strong>Upload</strong> · max 50 MB</span>
              </div>
            </div>
          </div>
        </div>
        <div className="card" style={{padding:'18px 22px', alignSelf:'start'}}>
          <div className="u-eyebrow" style={{marginBottom: 10}}>On this page</div>
          <div style={{display:'flex', flexDirection:'column', gap: 6, fontSize: 13}}>
            <a>Entering the cleanroom</a>
            <a>Chemical handling</a>
            <a>Emergency contacts</a>
          </div>
        </div>
      </div>
    </div>
  );
}

function A_Booking({ setScreen }) {
  const hours = Array.from({length: 12}, (_,i) => i+8);
  const [bookOpen, setBookOpen] = React.useState(false);
  const [prefill, setPrefill] = React.useState(null);
  return (
    <div>
      <A_BookSlotModal open={bookOpen} onClose={()=>setBookOpen(false)} prefill={prefill} onSave={()=>{}}/>
      <div className="topbar">
        <div className="topbar__left"><h1>Equipment Booking</h1><div className="crumbs">Friday, April 19, 2026 · Today</div></div>
        <div className="topbar__actions">
          <button className="btn btn--ghost">‹ Prev</button>
          <input type="date" defaultValue="2026-04-19" style={{padding:'8px 10px', borderRadius: 8, border:'1px solid var(--border-subtle)', background:'var(--surface-default)', color:'var(--text-strong)', minHeight: 38}}/>
          <button className="btn btn--ghost">Next ›</button>
          <button className="btn btn--primary" onClick={()=>{ setPrefill(null); setBookOpen(true); }}><Ic.Plus/> Book Slot</button>
        </div>
      </div>
      <div className="toolbar">
        <div className="filter-chip active">All equipment</div>
        <div className="filter-chip">My bookings</div>
        <div className="filter-chip">Available now</div>
        <div style={{flex:1}}/>
        <div style={{fontSize:12.5, color:'var(--text-muted)'}}>Click a cell to book · drag to create a range</div>
      </div>
      <div className="timeline-wrap">
        <div className="timeline-wrap__scroll">
          <div className="tline">
            <div className="tline-head">
              <div className="tline-head__left">Equipment</div>
              <div className="tline-hours">{hours.map(h=><div key={h} className="tline-hour">{h}:00</div>)}</div>
            </div>
            {EQUIPMENT.map(eq => (
              <div className="tline-row" key={eq.id}>
                <div className="tline-row__left"><span className="tline-row__dot"/>{eq.name}</div>
                <div className="tline-lane">
                  <div className="slots">{hours.map(h => <div key={h} className="slot" onClick={()=>{ setPrefill({eq: eq.id, start: h, end: h+1}); setBookOpen(true); }}/>)}</div>
                  {BOOKINGS.filter(b=>b.eq===eq.id).map((b,i)=>{
                    const left=(b.start-8)*80, w=(b.end-b.start)*80;
                    return <div key={i} className={'tline-block'+(b.own?' tline-block--own':'')} style={{left, width:w-4}}>
                      <div>{b.user}</div><small>{b.purpose}</small>
                    </div>;
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function A_Tokens() {
  return (
    <div>
      <div className="topbar"><div className="topbar__left"><h1>API Tokens</h1><div className="crumbs">Personal access tokens for programmatic access</div></div></div>
      <div style={{display:'grid', gridTemplateColumns:'1fr 2fr', gap: 18}}>
        <div className="card form-card">
          <h3 style={{margin:'0 0 4px', fontSize: 15, color:'var(--color-primary)'}}>Generate new token</h3>
          <p style={{fontSize:13, color:'var(--text-muted)', margin: '0 0 16px'}}>Tokens have full account permissions. Keep them secret.</p>
          <div className="field"><label>Token name</label><input placeholder="e.g. Mobile app"/></div>
          <div className="field"><label>Expiration</label><select><option>30 days</option><option>90 days</option><option>1 year</option><option>Never</option></select></div>
          <button className="btn btn--primary" style={{width:'100%'}}>Generate</button>
        </div>
        <div className="card" style={{padding: 0}}>
          <div style={{padding: '14px 20px', borderBottom: '1px solid var(--border-subtle)'}}>
            <strong style={{fontSize: 14}}>Existing tokens ({TOKENS.length})</strong>
          </div>
          <table className="data-table" style={{border:'none', borderRadius: 0}}>
            <thead><tr><th>Name</th><th>Created</th><th>Last used</th><th>Status</th><th></th></tr></thead>
            <tbody>
              {TOKENS.map(t=>(
                <tr key={t.name}>
                  <td><strong>{t.name}</strong></td>
                  <td style={{fontFamily:'var(--font-family-mono)', fontSize:12, color:'var(--text-muted)'}}>{t.created}</td>
                  <td style={{fontFamily:'var(--font-family-mono)', fontSize:12, color:'var(--text-muted)'}}>{t.used}</td>
                  <td>{t.status==='active' ? <span className="status-chip status-chip--active"><span className="dot"/>Active</span> : <span className="status-chip status-chip--archived"><span className="dot"/>Revoked</span>}</td>
                  <td>{t.status==='active' && <button className="btn btn--danger btn--sm">Revoke</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="card" style={{marginTop: 18, padding: '18px 22px'}}>
        <div className="u-eyebrow" style={{marginBottom: 10}}>Usage</div>
        <p style={{margin:'0 0 10px', fontSize: 13, color: 'var(--text-muted)'}}>Send the token in the <code>Authorization</code> header on every API request.</p>
        <pre style={{margin: 0, background:'var(--surface-subtle)', padding: 12, borderRadius: 8, fontSize: 13}}>Authorization: Bearer &lt;your-token&gt;</pre>
      </div>
    </div>
  );
}

function A_Admin() {
  const [groupOpen, setGroupOpen] = React.useState(false);
  const [equipOpen, setEquipOpen] = React.useState(false);
  const [editUser, setEditUser] = React.useState(null);
  const [extraGroups, setExtraGroups] = React.useState([]);
  const [extraEquip, setExtraEquip] = React.useState([]);
  return (
    <div>
      <div className="topbar"><div className="topbar__left"><h1>Admin Panel</h1><div className="crumbs">Manage users, groups, and equipment access</div></div></div>
      <div className="admin-grid" style={{display:'grid', gridTemplateColumns:'1fr 2fr', gap: 18, marginBottom: 18}}>
        <div className="card" style={{padding: 0}}>
          <div style={{padding:'14px 20px', borderBottom:'1px solid var(--border-subtle)', display:'flex', justifyContent:'space-between', alignItems:'center'}}>
            <strong style={{fontSize:14}}>User Groups</strong><button className="btn btn--secondary btn--sm" onClick={()=>setGroupOpen(true)}><Ic.Plus/>Add</button>
          </div>
          {['Barbaros', 'Ahmet', 'NewGroup', ...extraGroups].map(g => (
            <div key={g} style={{padding:'12px 20px', display:'flex', justifyContent:'space-between', alignItems:'center', borderBottom:'1px solid var(--border-subtle)'}}>
              <div><strong style={{fontSize:14}}>{g}</strong><div style={{fontSize:12, color:'var(--text-muted)'}}>{g==='Barbaros'?3:g==='Ahmet'?2:1} members</div></div>
              <button className="btn btn--ghost btn--sm"><Ic.Trash/></button>
            </div>
          ))}
        </div>
        <div className="card" style={{padding: 0}}>
          <div style={{padding:'14px 20px', borderBottom:'1px solid var(--border-subtle)', display:'flex', justifyContent:'space-between'}}>
            <strong style={{fontSize:14}}>Equipment Catalogue</strong><button className="btn btn--secondary btn--sm" onClick={()=>setEquipOpen(true)}><Ic.Plus/>Add Equipment</button>
          </div>
          <table className="data-table" style={{border:'none', borderRadius: 0}}>
            <thead><tr><th>Name</th><th>Group access</th><th>Status</th><th></th></tr></thead>
            <tbody>
              {EQUIPMENT.map(eq => (
                <tr key={eq.id}><td><strong>{eq.name}</strong></td>
                  <td><span className="status-chip status-chip--progress">Barbaros</span> <span className="status-chip status-chip--progress">Ahmet</span></td>
                  <td><span className="status-chip status-chip--active"><span className="dot"/>Online</span></td>
                  <td><button className="btn btn--ghost btn--sm">Usage</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="card" style={{padding: 0}}>
        <div style={{padding:'14px 20px', borderBottom:'1px solid var(--border-subtle)', display:'flex', justifyContent: 'space-between'}}>
          <strong style={{fontSize:14}}>User Management ({USERS.length})</strong>
          <div className="search" style={{minWidth: 220, minHeight: 30}}><Ic.Search/><input placeholder="Search users…"/></div>
        </div>
        <table className="data-table" style={{border:'none', borderRadius: 0}}>
          <thead><tr><th>User</th><th>Group</th><th>Status</th><th>Admin</th><th>Equipment access</th><th></th></tr></thead>
          <tbody>
            {USERS.map(u=>(
              <tr key={u.user}>
                <td><div style={{display:'flex', alignItems:'center', gap: 10}}><div className="avatar" style={{width: 28, height: 28, fontSize: 12}}>{u.user[0].toUpperCase()}</div><strong>{u.user}</strong></div></td>
                <td><select defaultValue={u.group} style={{minHeight: 30, padding: '4px 10px', fontSize: 13}}>{['Barbaros','Ahmet','NewGroup','No group'].map(g=><option key={g}>{g}</option>)}</select></td>
                <td>{u.status==='approved' ? <span className="status-chip status-chip--active"><span className="dot"/>Approved</span> : <span className="status-chip status-chip--pending"><span className="dot"/>Pending</span>}</td>
                <td>{u.admin ? <span className="status-chip status-chip--progress">Admin</span> : '—'}</td>
                <td><span className="status-chip status-chip--archived">4 tools</span></td>
                <td><button className="btn btn--ghost btn--sm" onClick={()=>setEditUser(u)}><Ic.Edit/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <A_AddGroupModal open={groupOpen} onClose={()=>setGroupOpen(false)} onSave={g => setExtraGroups(s => [...s, g.name])}/>
      <A_AddEquipmentModal open={equipOpen} onClose={()=>setEquipOpen(false)} onSave={e => setExtraEquip(s => [...s, e.name])}/>
      <A_EditUserModal open={!!editUser} user={editUser} onClose={()=>setEditUser(null)} onSave={()=>{}}/>
    </div>
  );
}

function DirectionA({ screen, setScreen, activeSample, setActiveSample }) {
  return (
    <div className="dirA">
      <div className="app-shell">
        <A_Sidebar screen={screen} setScreen={setScreen}/>
        <div className="main">
          {screen === 'dashboard' && <A_Dashboard setScreen={setScreen} setActiveSample={setActiveSample}/>}
          {screen === 'samples' && <A_Samples setScreen={setScreen} setActiveSample={setActiveSample}/>}
          {screen === 'detail' && <A_SampleDetail sample={activeSample} setScreen={setScreen}/>}
          {screen === 'new' && <A_New setScreen={setScreen}/>}
          {screen === 'wiki' && <A_Wiki setScreen={setScreen}/>}
          {screen === 'wikiArticle' && <A_WikiArticle setScreen={setScreen}/>}
          {screen === 'booking' && <A_Booking setScreen={setScreen}/>}
          {screen === 'tokens' && <A_Tokens/>}
          {screen === 'admin' && <A_Admin/>}
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { DirectionA });
